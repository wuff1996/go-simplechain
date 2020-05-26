package db

import (
	"fmt"
	"math/big"

	"github.com/simplechain-org/go-simplechain/common"
	"github.com/simplechain-org/go-simplechain/common/math"
	cc "github.com/simplechain-org/go-simplechain/cross/core"
	"github.com/simplechain-org/go-simplechain/log"

	"github.com/asdine/storm/v3"
	"github.com/asdine/storm/v3/q"
)

type indexDB struct {
	chainID *big.Int
	root    *storm.DB // root db of stormDB
	db      storm.Node
	cache   *IndexDbCache
}

type FieldName = string

const (
	PK               FieldName = "PK"
	CtxIdIndex       FieldName = "CtxId"
	TxHashIndex      FieldName = "TxHash"
	PriceIndex       FieldName = "Price"
	StatusField      FieldName = "Status"
	FromField        FieldName = "From"
	DestinationValue FieldName = "DestinationValue"
	BlockNumField    FieldName = "BlockNum"
)

func NewIndexDB(chainID *big.Int, rootDB *storm.DB, cacheSize uint64) *indexDB {
	dbName := "chain" + chainID.String()
	log.Info("New IndexDB", "dbName", dbName, "cacheSize", cacheSize)
	return &indexDB{
		chainID: chainID,
		db:      rootDB.From(dbName).WithBatch(true),
		cache:   newIndexDbCache(int(cacheSize)),
	}
}

func (d *indexDB) ChainID() *big.Int {
	return d.chainID
}

func (d *indexDB) Count(filter ...q.Matcher) int {
	count, _ := d.db.Select(filter...).Count(&CrossTransactionIndexed{})
	return count
}

func (d *indexDB) Load() error {
	return nil
}

func (d *indexDB) Height() uint64 {
	var ctxs []*CrossTransactionIndexed
	if err := d.db.AllByIndex(BlockNumField, &ctxs, storm.Limit(1), storm.Reverse()); err != nil || len(ctxs) == 0 {
		return 0
	}
	return ctxs[0].BlockNum
}

func (d *indexDB) Repair() error {
	return d.db.ReIndex(&CrossTransactionIndexed{})
}

func (d *indexDB) Clean() error {
	return d.db.Drop(&CrossTransactionIndexed{})
}

func (d *indexDB) Close() error {
	return d.db.Commit()
}

func (d *indexDB) Write(ctx *cc.CrossTransactionWithSignatures) error {
	old, err := d.get(ctx.ID())
	if old != nil {
		if old.BlockHash != ctx.BlockHash() {
			return ErrCtxDbFailure{err: fmt.Errorf("blockchain reorg, txID:%s, old:%s, new:%s",
				ctx.ID(), old.BlockHash.String(), ctx.BlockHash().String())}
		}
		return nil
	}

	persist := NewCrossTransactionIndexed(ctx)
	err = d.db.Save(persist)
	if err != nil {
		return ErrCtxDbFailure{fmt.Sprintf("Write:%s save fail", ctx.ID().String()), err}
	}

	if d.cache != nil {
		d.cache.Put(CtxIdIndex, ctx.ID(), persist)
	}
	return nil
}

func (d *indexDB) Read(ctxId common.Hash) (*cc.CrossTransactionWithSignatures, error) {
	ctx, err := d.get(ctxId)
	if err != nil {
		return nil, err
	}
	return ctx.ToCrossTransaction(), nil
}

func (d *indexDB) One(field FieldName, key interface{}) *cc.CrossTransactionWithSignatures {
	if d.cache != nil && d.cache.Has(field, key) {
		return d.cache.Get(field, key).ToCrossTransaction()
	}
	var ctx CrossTransactionIndexed
	if err := d.db.One(field, key, &ctx); err != nil {
		return nil
	}
	if d.cache != nil {
		d.cache.Put(field, key, &ctx)
	}
	return ctx.ToCrossTransaction()
}

func (d *indexDB) get(ctxId common.Hash) (*CrossTransactionIndexed, error) {
	if d.cache != nil && d.cache.Has(CtxIdIndex, ctxId) {
		return d.cache.Get(CtxIdIndex, ctxId), nil
	}

	var ctx CrossTransactionIndexed
	if err := d.db.One(CtxIdIndex, ctxId, &ctx); err != nil {
		return nil, ErrCtxDbFailure{fmt.Sprintf("get ctx:%s failed", ctxId.String()), err}
	}

	if d.cache != nil {
		d.cache.Put(CtxIdIndex, ctxId, &ctx)
	}

	return &ctx, nil
}

func (d *indexDB) Update(id common.Hash, updater func(ctx *CrossTransactionIndexed)) error {
	ctx, err := d.get(id)
	if err != nil {
		return err
	}
	updater(ctx) // updater should never be allowed to modify PK or ctxID!
	if err := d.db.Update(ctx); err != nil {
		return ErrCtxDbFailure{"Update save fail", err}
	}
	if d.cache != nil {
		d.cache.Put(CtxIdIndex, id, ctx)
	}
	return nil
}

func (d *indexDB) Has(id common.Hash) bool {
	_, err := d.get(id)
	return err == nil
}

func (d *indexDB) Range(pageSize int, startCtxID, endCtxID *common.Hash) []*cc.CrossTransactionWithSignatures {
	var (
		min, max uint64 = 0, math.MaxUint64
		results  []*cc.CrossTransactionWithSignatures
		list     []*CrossTransactionIndexed
	)

	if startCtxID != nil {
		start, err := d.get(*startCtxID)
		if err != nil {
			return nil
		}
		min = start.PK + 1
	}
	if endCtxID != nil {
		end, err := d.get(*endCtxID)
		if err != nil {
			return nil
		}
		max = end.PK
	}

	if err := d.db.Range(PK, min, max, &list, storm.Limit(pageSize)); err != nil {
		log.Debug("range return no result", "startID", startCtxID, "endID", endCtxID, "minPK", min, "maxPK", max, "err", err)
		return nil
	}

	results = make([]*cc.CrossTransactionWithSignatures, len(list))
	for i, ctx := range list {
		results[i] = ctx.ToCrossTransaction()
	}
	return results
}

func (d *indexDB) Query(pageSize int, startPage int, orderBy []FieldName, reverse bool, filter ...q.Matcher) []*cc.CrossTransactionWithSignatures {
	if pageSize > 0 && startPage <= 0 {
		return nil
	}
	var ctxs []*CrossTransactionIndexed
	query := d.db.Select(filter...)
	if len(orderBy) > 0 {
		query.OrderBy(orderBy...)
	}
	if reverse {
		query.Reverse()
	}
	if pageSize > 0 {
		query.Limit(pageSize).Skip(pageSize * (startPage - 1))
	}
	query.Find(&ctxs)

	results := make([]*cc.CrossTransactionWithSignatures, len(ctxs))
	for i, ctx := range ctxs {
		results[i] = ctx.ToCrossTransaction()
	}
	return results
}

func (d *indexDB) RangeByNumber(begin, end uint64, pageSize int) []*cc.CrossTransactionWithSignatures {
	var ctxs []*CrossTransactionIndexed
	d.db.Range(BlockNumField, begin, end, &ctxs, storm.Limit(pageSize))
	if ctxs == nil {
		return nil
	}
	//把最后一笔ctx所在高度的所有ctx取出来
	var lasts []*CrossTransactionIndexed
	d.db.Find(BlockNumField, ctxs[len(ctxs)-1].BlockNum, &lasts)
	for i, tx := range ctxs {
		if tx.BlockNum == ctxs[len(ctxs)-1].BlockNum {
			ctxs = ctxs[:i]
			break
		}
	}
	ctxs = append(ctxs, lasts...)

	results := make([]*cc.CrossTransactionWithSignatures, len(ctxs))
	for i, ctx := range ctxs {
		results[i] = ctx.ToCrossTransaction()
	}
	return results
}