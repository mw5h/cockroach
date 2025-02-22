// Copyright 2016 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package builtins

import (
	"context"

	"github.com/cockroachdb/cockroach/pkg/sql/pgwire/pgcode"
	"github.com/cockroachdb/cockroach/pkg/sql/pgwire/pgerror"
	"github.com/cockroachdb/cockroach/pkg/sql/sem/eval"
	"github.com/cockroachdb/cockroach/pkg/sql/sem/tree"
	"github.com/cockroachdb/cockroach/pkg/sql/sem/volatility"
	"github.com/cockroachdb/cockroach/pkg/sql/types"
)

func init() {
	for k, v := range windows {
		const enforceClass = true
		registerBuiltin(k, v, tree.WindowClass, enforceClass)
	}
}

// windows are a special class of builtin functions that can only be applied
// as window functions using an OVER clause.
// See `windowFuncHolder` in the sql package.
var windows = map[string]builtinDefinition{
	"row_number": makeBuiltin(tree.FunctionProperties{},
		makeWindowOverload(
			tree.ParamTypes{},
			types.Int,
			newRowNumberWindow,
			"Calculates the number of the current row within its partition, counting from 1.",
			volatility.Immutable,
		),
	),
	"rank": makeBuiltin(tree.FunctionProperties{},
		makeWindowOverload(
			tree.ParamTypes{},
			types.Int,
			newRankWindow,
			"Calculates the rank of the current row with gaps; same as row_number of its first peer.",
			volatility.Immutable,
		),
	),
	"dense_rank": makeBuiltin(tree.FunctionProperties{},
		makeWindowOverload(
			tree.ParamTypes{},
			types.Int,
			newDenseRankWindow,
			"Calculates the rank of the current row without gaps; this function counts peer groups.",
			volatility.Immutable,
		),
	),
	"percent_rank": makeBuiltin(tree.FunctionProperties{},
		makeWindowOverload(
			tree.ParamTypes{},
			types.Float,
			newPercentRankWindow,
			"Calculates the relative rank of the current row: (rank - 1) / (total rows - 1).",
			volatility.Immutable,
		),
	),
	"cume_dist": makeBuiltin(tree.FunctionProperties{},
		makeWindowOverload(
			tree.ParamTypes{},
			types.Float,
			newCumulativeDistWindow,
			"Calculates the relative rank of the current row: "+
				"(number of rows preceding or peer with current row) / (total rows).",
			volatility.Immutable,
		),
	),
	"ntile": makeBuiltin(tree.FunctionProperties{},
		makeWindowOverload(
			tree.ParamTypes{{Name: "n", Typ: types.Int}},
			types.Int,
			newNtileWindow,
			"Calculates an integer ranging from 1 to `n`, dividing the partition as equally as possible.",
			volatility.Immutable,
		),
	),
	"lag": collectOverloads(
		tree.FunctionProperties{},
		types.Scalar,
		func(t *types.T) tree.Overload {
			return makeWindowOverload(
				tree.ParamTypes{{Name: "val", Typ: t}},
				t,
				makeLeadLagWindowConstructor(false, false, false),
				"Returns `val` evaluated at the previous row within current row's partition; "+
					"if there is no such row, instead returns null.",
				volatility.Immutable,
			)
		},
		func(t *types.T) tree.Overload {
			return makeWindowOverload(
				tree.ParamTypes{{Name: "val", Typ: t}, {Name: "n", Typ: types.Int}},
				t,
				makeLeadLagWindowConstructor(false, true, false),
				"Returns `val` evaluated at the row that is `n` rows before the current row within its partition; "+
					"if there is no such row, instead returns null. `n` is evaluated with respect to the current row.",
				volatility.Immutable,
			)
		},
		// TODO(nvanbenschoten): We still have no good way to represent two parameters that
		// can be any types but must be the same (eg. lag(T, Int, T)).
		func(t *types.T) tree.Overload {
			return makeWindowOverload(
				tree.ParamTypes{
					{Name: "val", Typ: t}, {Name: "n", Typ: types.Int}, {Name: "default", Typ: t},
				},
				t,
				makeLeadLagWindowConstructor(false, true, true),
				"Returns `val` evaluated at the row that is `n` rows before the current row within its partition; "+
					"if there is no such, row, instead returns `default` (which must be of the same type as `val`). "+
					"Both `n` and `default` are evaluated with respect to the current row.",
				volatility.Immutable,
			)
		},
	),
	"lead": collectOverloads(tree.FunctionProperties{}, types.Scalar,
		func(t *types.T) tree.Overload {
			return makeWindowOverload(
				tree.ParamTypes{{Name: "val", Typ: t}},
				t,
				makeLeadLagWindowConstructor(true, false, false),
				"Returns `val` evaluated at the following row within current row's partition; "+""+
					"if there is no such row, instead returns null.",
				volatility.Immutable,
			)
		},
		func(t *types.T) tree.Overload {
			return makeWindowOverload(
				tree.ParamTypes{{Name: "val", Typ: t}, {Name: "n", Typ: types.Int}},
				t,
				makeLeadLagWindowConstructor(true, true, false),
				"Returns `val` evaluated at the row that is `n` rows after the current row within its partition; "+
					"if there is no such row, instead returns null. `n` is evaluated with respect to the current row.",
				volatility.Immutable,
			)
		},
		func(t *types.T) tree.Overload {
			return makeWindowOverload(
				tree.ParamTypes{
					{Name: "val", Typ: t}, {Name: "n", Typ: types.Int}, {Name: "default", Typ: t},
				},
				t,
				makeLeadLagWindowConstructor(true, true, true),
				"Returns `val` evaluated at the row that is `n` rows after the current row within its partition; "+
					"if there is no such, row, instead returns `default` (which must be of the same type as `val`). "+
					"Both `n` and `default` are evaluated with respect to the current row.",
				volatility.Immutable,
			)
		},
	),
	"first_value": collectOverloads(
		tree.FunctionProperties{},
		types.Scalar,
		func(t *types.T) tree.Overload {
			return makeWindowOverload(
				tree.ParamTypes{{Name: "val", Typ: t}},
				t,
				newFirstValueWindow,
				"Returns `val` evaluated at the row that is the first row of the window frame.",
				volatility.Immutable,
			)
		}),
	"last_value": collectOverloads(
		tree.FunctionProperties{},
		types.Scalar,
		func(t *types.T) tree.Overload {
			return makeWindowOverload(
				tree.ParamTypes{{Name: "val", Typ: t}},
				t,
				newLastValueWindow,
				"Returns `val` evaluated at the row that is the last row of the window frame.",
				volatility.Immutable,
			)
		}),
	"nth_value": collectOverloads(tree.FunctionProperties{}, types.Scalar,
		func(t *types.T) tree.Overload {
			return makeWindowOverload(
				tree.ParamTypes{
					{Name: "val", Typ: t}, {Name: "n", Typ: types.Int},
				},
				t,
				newNthValueWindow,
				"Returns `val` evaluated at the row that is the `n`th row of the window frame (counting from 1); "+
					"null if no such row.",
				volatility.Immutable,
			)
		}),
}

func makeWindowOverload(
	in tree.ParamTypes, ret *types.T, f eval.WindowOverload, info string, volatility volatility.V,
) tree.Overload {
	return tree.Overload{
		Types:      in,
		ReturnType: tree.FixedReturnType(ret),
		WindowFunc: f,
		Class:      tree.WindowClass,
		Info:       info,
		Volatility: volatility,
	}
}

var _ eval.WindowFunc = &aggregateWindowFunc{}
var _ eval.WindowFunc = &framableAggregateWindowFunc{}
var _ eval.WindowFunc = &rowNumberWindow{}
var _ eval.WindowFunc = &rankWindow{}
var _ eval.WindowFunc = &denseRankWindow{}
var _ eval.WindowFunc = &percentRankWindow{}
var _ eval.WindowFunc = &cumulativeDistWindow{}
var _ eval.WindowFunc = &ntileWindow{}
var _ eval.WindowFunc = &leadLagWindow{}
var _ eval.WindowFunc = &firstValueWindow{}
var _ eval.WindowFunc = &lastValueWindow{}
var _ eval.WindowFunc = &nthValueWindow{}

// aggregateWindowFunc aggregates over the current row's window frame, using
// the internal eval.AggregateFunc to perform the aggregation.
//
// INVARIANT: the rows within a window frame are always processed in the same
// order, regardless of whether the user specified an ordering. This means that
// two rows with the exact same frame will produce the same result for a given
// aggregation.
type aggregateWindowFunc struct {
	agg     eval.AggregateFunc
	peerRes tree.Datum
	// peerFrameStartIdx and peerFrameEndIdx indicate the boundaries of the
	// window frame over which peerRes was computed.
	peerFrameStartIdx, peerFrameEndIdx int
}

// NewAggregateWindowFunc creates a constructor of aggregateWindowFunc
// with agg initialized by provided aggConstructor.
func NewAggregateWindowFunc(
	aggConstructor func(*eval.Context, tree.Datums) eval.AggregateFunc,
) func(*eval.Context) eval.WindowFunc {
	return func(evalCtx *eval.Context) eval.WindowFunc {
		return &aggregateWindowFunc{agg: aggConstructor(evalCtx, nil /* arguments */)}
	}
}

func (w *aggregateWindowFunc) Compute(
	ctx context.Context, evalCtx *eval.Context, wfr *eval.WindowFrameRun,
) (tree.Datum, error) {
	if !wfr.FirstInPeerGroup() && wfr.Frame.DefaultFrameExclusion() {
		return w.peerRes, nil
	}

	// Accumulate all values in the peer group at the same time, as these
	// must return the same value.
	peerGroupRowCount := wfr.PeerHelper.GetRowCount(wfr.CurRowPeerGroupNum)
	for i := 0; i < peerGroupRowCount; i++ {
		if skipped, err := wfr.IsRowSkipped(ctx, wfr.RowIdx+i); err != nil {
			return nil, err
		} else if skipped {
			continue
		}
		args, err := wfr.ArgsWithRowOffset(ctx, i)
		if err != nil {
			return nil, err
		}
		var value tree.Datum
		var others tree.Datums
		// COUNT_ROWS takes no arguments.
		if len(args) > 0 {
			value = args[0]
			others = args[1:]
		}
		if err := w.agg.Add(ctx, value, others...); err != nil {
			return nil, err
		}
	}

	// Retrieve the value for the entire peer group, save it, and return it.
	peerRes, err := w.agg.Result()
	if err != nil {
		return nil, err
	}
	w.peerRes = peerRes
	w.peerFrameStartIdx = wfr.RowIdx
	w.peerFrameEndIdx = wfr.RowIdx + peerGroupRowCount
	return w.peerRes, nil
}

// Reset implements eval.WindowFunc interface.
func (w *aggregateWindowFunc) Reset(ctx context.Context) {
	w.agg.Reset(ctx)
	w.peerRes = nil
	w.peerFrameStartIdx = 0
	w.peerFrameEndIdx = 0
}

func (w *aggregateWindowFunc) Close(ctx context.Context, _ *eval.Context) {
	w.agg.Close(ctx)
}

// ShouldReset sets shouldReset to true if w is framableAggregateWindowFunc.
func ShouldReset(w eval.WindowFunc) {
	if f, ok := w.(*framableAggregateWindowFunc); ok {
		f.shouldReset = true
	}
}

// framableAggregateWindowFunc is a wrapper around aggregateWindowFunc that allows
// to reset the aggregate by creating a new instance via a provided constructor.
// shouldReset indicates whether the resetting behavior is desired.
type framableAggregateWindowFunc struct {
	agg            *aggregateWindowFunc
	aggConstructor func(*eval.Context, tree.Datums) eval.AggregateFunc
	shouldReset    bool
}

func newFramableAggregateWindow(
	agg eval.AggregateFunc, aggConstructor func(*eval.Context, tree.Datums) eval.AggregateFunc,
) eval.WindowFunc {
	// jsonObjectAggregate is a special aggregate function because its
	// implementation assumes that once Result is called, the returned
	// object is immutable and calls to Add will result in a panic. To go
	// around this limitation, we make sure that the function is reset for
	// each row regardless of the window frame.
	_, shouldReset := agg.(*jsonObjectAggregate)
	return &framableAggregateWindowFunc{
		agg:            &aggregateWindowFunc{agg: agg, peerRes: tree.DNull},
		aggConstructor: aggConstructor,
		shouldReset:    shouldReset,
	}
}

func (w *framableAggregateWindowFunc) Compute(
	ctx context.Context, evalCtx *eval.Context, wfr *eval.WindowFrameRun,
) (tree.Datum, error) {
	if wfr.FullPartitionIsInWindow() {
		// Full partition is always inside of the window, and aggregations will
		// return the same result for all of the rows, so we're actually performing
		// the aggregation once, on the first row, and reuse the result for all
		// other rows.
		if wfr.RowIdx > 0 {
			return w.agg.peerRes, nil
		}
	}
	frameStartIdx, err := wfr.FrameStartIdx(ctx, evalCtx)
	if err != nil {
		return nil, err
	}
	frameEndIdx, err := wfr.FrameEndIdx(ctx, evalCtx)
	if err != nil {
		return nil, err
	}
	if !wfr.FirstInPeerGroup() && wfr.Frame.DefaultFrameExclusion() {
		// The concept of window framing takes precedence over the concept of
		// peers - although we calculated the result for one of the peers of the
		// current row, it is possible for that peer to have a different window
		// frame, so we check that.
		if frameStartIdx == w.agg.peerFrameStartIdx && frameEndIdx == w.agg.peerFrameEndIdx {
			// The window frame is the same, so we return already calculated result.
			return w.agg.peerRes, nil
		}
		// The window frame is different, so we need to recalculate the result.
	}
	if !w.shouldReset {
		// We should not reset, so we will use the same aggregateWindowFunc.
		return w.agg.Compute(ctx, evalCtx, wfr)
	}

	// We should reset the aggregate, so we dispose of the old aggregate function
	// and construct a new one for the computation.
	w.agg.Close(ctx, evalCtx)
	// No arguments are passed into the aggConstructor and they are instead passed
	// in during the call to add().
	*w.agg = aggregateWindowFunc{
		agg:     w.aggConstructor(evalCtx, nil /* arguments */),
		peerRes: tree.DNull,
	}

	// Accumulate all values in the window frame.
	for i := frameStartIdx; i < frameEndIdx; i++ {
		if skipped, err := wfr.IsRowSkipped(ctx, i); err != nil {
			return nil, err
		} else if skipped {
			continue
		}
		args, err := wfr.ArgsByRowIdx(ctx, i)
		if err != nil {
			return nil, err
		}
		var value tree.Datum
		var others tree.Datums
		// COUNT_ROWS takes no arguments.
		if len(args) > 0 {
			value = args[0]
			others = args[1:]
		}
		if err := w.agg.agg.Add(ctx, value, others...); err != nil {
			return nil, err
		}
	}

	// Retrieve the value for the entire peer group, save it, and return it.
	peerRes, err := w.agg.agg.Result()
	if err != nil {
		return nil, err
	}
	w.agg.peerRes = peerRes
	w.agg.peerFrameStartIdx = frameStartIdx
	w.agg.peerFrameEndIdx = frameEndIdx
	return w.agg.peerRes, nil
}

// Reset implements eval.WindowFunc interface.
func (w *framableAggregateWindowFunc) Reset(ctx context.Context) {
	w.agg.Reset(ctx)
}

func (w *framableAggregateWindowFunc) Close(ctx context.Context, evalCtx *eval.Context) {
	w.agg.Close(ctx, evalCtx)
}

// rowNumberWindow computes the number of the current row within its partition,
// counting from 1.
type rowNumberWindow struct{}

func newRowNumberWindow([]*types.T, *eval.Context) eval.WindowFunc {
	return &rowNumberWindow{}
}

func (rowNumberWindow) Compute(
	_ context.Context, _ *eval.Context, wfr *eval.WindowFrameRun,
) (tree.Datum, error) {
	return tree.NewDInt(tree.DInt(wfr.RowIdx + 1 /* one-indexed */)), nil
}

// Reset implements eval.WindowFunc interface.
func (rowNumberWindow) Reset(context.Context) {}

func (rowNumberWindow) Close(context.Context, *eval.Context) {}

// rankWindow computes the rank of the current row with gaps.
type rankWindow struct {
	peerRes *tree.DInt
}

func newRankWindow([]*types.T, *eval.Context) eval.WindowFunc {
	return &rankWindow{}
}

func (w *rankWindow) Compute(
	_ context.Context, _ *eval.Context, wfr *eval.WindowFrameRun,
) (tree.Datum, error) {
	if wfr.FirstInPeerGroup() {
		w.peerRes = tree.NewDInt(tree.DInt(wfr.Rank()))
	}
	return w.peerRes, nil
}

// Reset implements eval.WindowFunc interface.
func (w *rankWindow) Reset(context.Context) {
	w.peerRes = nil
}

func (w *rankWindow) Close(context.Context, *eval.Context) {}

// denseRankWindow computes the rank of the current row without gaps (it counts peer groups).
type denseRankWindow struct {
	denseRank int
	peerRes   *tree.DInt
}

func newDenseRankWindow([]*types.T, *eval.Context) eval.WindowFunc {
	return &denseRankWindow{}
}

func (w *denseRankWindow) Compute(
	_ context.Context, _ *eval.Context, wfr *eval.WindowFrameRun,
) (tree.Datum, error) {
	if wfr.FirstInPeerGroup() {
		w.denseRank++
		w.peerRes = tree.NewDInt(tree.DInt(w.denseRank))
	}
	return w.peerRes, nil
}

// Reset implements eval.WindowFunc interface.
func (w *denseRankWindow) Reset(context.Context) {
	w.denseRank = 0
	w.peerRes = nil
}

func (w *denseRankWindow) Close(context.Context, *eval.Context) {}

// percentRankWindow computes the relative rank of the current row using:
//
//	(rank - 1) / (total rows - 1)
type percentRankWindow struct {
	peerRes *tree.DFloat
}

func newPercentRankWindow([]*types.T, *eval.Context) eval.WindowFunc {
	return &percentRankWindow{}
}

var dfloatZero = tree.NewDFloat(0)

func (w *percentRankWindow) Compute(
	_ context.Context, _ *eval.Context, wfr *eval.WindowFrameRun,
) (tree.Datum, error) {
	// Return zero if there's only one row, per spec.
	if wfr.PartitionSize() <= 1 {
		return dfloatZero, nil
	}

	if wfr.FirstInPeerGroup() {
		// (rank - 1) / (total rows - 1)
		w.peerRes = tree.NewDFloat(tree.DFloat(wfr.Rank()-1) / tree.DFloat(wfr.PartitionSize()-1))
	}
	return w.peerRes, nil
}

// Reset implements eval.WindowFunc interface.
func (w *percentRankWindow) Reset(context.Context) {
	w.peerRes = nil
}

func (w *percentRankWindow) Close(context.Context, *eval.Context) {}

// cumulativeDistWindow computes the relative rank of the current row using:
//
//	(number of rows preceding or peer with current row) / (total rows)
type cumulativeDistWindow struct {
	peerRes *tree.DFloat
}

func newCumulativeDistWindow([]*types.T, *eval.Context) eval.WindowFunc {
	return &cumulativeDistWindow{}
}

func (w *cumulativeDistWindow) Compute(
	_ context.Context, _ *eval.Context, wfr *eval.WindowFrameRun,
) (tree.Datum, error) {
	if wfr.FirstInPeerGroup() {
		// (number of rows preceding or peer with current row) / (total rows)
		w.peerRes = tree.NewDFloat(tree.DFloat(wfr.DefaultFrameSize()) / tree.DFloat(wfr.PartitionSize()))
	}
	return w.peerRes, nil
}

// Reset implements eval.WindowFunc interface.
func (w *cumulativeDistWindow) Reset(context.Context) {
	w.peerRes = nil
}

func (w *cumulativeDistWindow) Close(context.Context, *eval.Context) {}

// ntileWindow computes an integer ranging from 1 to the argument value, dividing
// the partition as equally as possible.
type ntileWindow struct {
	ntile          *tree.DInt // current result
	curBucketCount int        // row number of current bucket
	boundary       int        // how many rows should be in the bucket
	remainder      int        // (total rows) % (bucket num)
}

func newNtileWindow([]*types.T, *eval.Context) eval.WindowFunc {
	return &ntileWindow{}
}

// ErrInvalidArgumentForNtile is thrown when the ntile function is given an
// argument less than or equal to zero.
var ErrInvalidArgumentForNtile = pgerror.Newf(
	pgcode.InvalidParameterValue, "argument of ntile() must be greater than zero")

func (w *ntileWindow) Compute(
	ctx context.Context, _ *eval.Context, wfr *eval.WindowFrameRun,
) (tree.Datum, error) {
	if w.ntile == nil {
		// If this is the first call to ntileWindow.Compute, set up the buckets.
		total := wfr.PartitionSize()

		args, err := wfr.Args(ctx)
		if err != nil {
			return nil, err
		}
		arg := args[0]
		if arg == tree.DNull {
			// per spec: If argument is the null value, then the result is the null value.
			return tree.DNull, nil
		}

		nbuckets := int(tree.MustBeDInt(arg))
		if nbuckets <= 0 {
			// per spec: If argument is less than or equal to 0, then an error is returned.
			return nil, ErrInvalidArgumentForNtile
		}

		w.ntile = tree.NewDInt(1)
		w.curBucketCount = 0
		w.boundary = total / nbuckets
		if w.boundary <= 0 {
			w.boundary = 1
		} else {
			// If the total number is not divisible, add 1 row to leading buckets.
			w.remainder = total % nbuckets
			if w.remainder != 0 {
				w.boundary++
			}
		}
	}

	w.curBucketCount++
	if w.boundary < w.curBucketCount {
		// Move to next ntile bucket.
		if w.remainder != 0 && int(*w.ntile) == w.remainder {
			w.remainder = 0
			w.boundary--
		}
		w.ntile = tree.NewDInt(*w.ntile + 1)
		w.curBucketCount = 1
	}
	return w.ntile, nil
}

// Reset implements eval.WindowFunc interface.
func (w *ntileWindow) Reset(context.Context) {
	w.boundary = 0
	w.curBucketCount = 0
	w.ntile = nil
	w.remainder = 0
}

func (w *ntileWindow) Close(context.Context, *eval.Context) {}

type leadLagWindow struct {
	forward     bool
	withOffset  bool
	withDefault bool
}

func newLeadLagWindow(forward, withOffset, withDefault bool) eval.WindowFunc {
	return &leadLagWindow{
		forward:     forward,
		withOffset:  withOffset,
		withDefault: withDefault,
	}
}

func makeLeadLagWindowConstructor(
	forward, withOffset, withDefault bool,
) func([]*types.T, *eval.Context) eval.WindowFunc {
	return func([]*types.T, *eval.Context) eval.WindowFunc {
		return newLeadLagWindow(forward, withOffset, withDefault)
	}
}

func (w *leadLagWindow) Compute(
	ctx context.Context, _ *eval.Context, wfr *eval.WindowFrameRun,
) (tree.Datum, error) {
	offset := 1
	if w.withOffset {
		args, err := wfr.Args(ctx)
		if err != nil {
			return nil, err
		}
		offsetArg := args[1]
		if offsetArg == tree.DNull {
			return tree.DNull, nil
		}
		offset = int(tree.MustBeDInt(offsetArg))
	}
	if !w.forward {
		offset *= -1
	}

	if targetRow := wfr.RowIdx + offset; targetRow < 0 || targetRow >= wfr.PartitionSize() {
		// Target row is out of the partition; supply default value if provided,
		// otherwise return NULL.
		if w.withDefault {
			args, err := wfr.Args(ctx)
			if err != nil {
				return nil, err
			}
			return args[2], nil
		}
		return tree.DNull, nil
	}

	args, err := wfr.ArgsWithRowOffset(ctx, offset)
	if err != nil {
		return nil, err
	}
	return args[0], nil
}

// Reset implements eval.WindowFunc interface.
func (w *leadLagWindow) Reset(context.Context) {}

func (w *leadLagWindow) Close(context.Context, *eval.Context) {}

// firstValueWindow returns value evaluated at the row that is the first row of the window frame.
type firstValueWindow struct{}

func newFirstValueWindow([]*types.T, *eval.Context) eval.WindowFunc {
	return &firstValueWindow{}
}

func (firstValueWindow) Compute(
	ctx context.Context, evalCtx *eval.Context, wfr *eval.WindowFrameRun,
) (tree.Datum, error) {
	frameStartIdx, err := wfr.FrameStartIdx(ctx, evalCtx)
	if err != nil {
		return nil, err
	}
	frameEndIdx, err := wfr.FrameEndIdx(ctx, evalCtx)
	if err != nil {
		return nil, err
	}
	for idx := frameStartIdx; idx < frameEndIdx; idx++ {
		if skipped, err := wfr.IsRowSkipped(ctx, idx); err != nil {
			return nil, err
		} else if !skipped {
			row, err := wfr.Rows.GetRow(ctx, idx)
			if err != nil {
				return nil, err
			}
			return row.GetDatum(int(wfr.ArgsIdxs[0]))
		}
	}
	// All rows are skipped from the frame, so it is empty, and we return NULL.
	return tree.DNull, nil
}

// Reset implements eval.WindowFunc interface.
func (firstValueWindow) Reset(context.Context) {}

func (firstValueWindow) Close(context.Context, *eval.Context) {}

// lastValueWindow returns value evaluated at the row that is the last row of the window frame.
type lastValueWindow struct{}

func newLastValueWindow([]*types.T, *eval.Context) eval.WindowFunc {
	return &lastValueWindow{}
}

func (lastValueWindow) Compute(
	ctx context.Context, evalCtx *eval.Context, wfr *eval.WindowFrameRun,
) (tree.Datum, error) {
	frameStartIdx, err := wfr.FrameStartIdx(ctx, evalCtx)
	if err != nil {
		return nil, err
	}
	frameEndIdx, err := wfr.FrameEndIdx(ctx, evalCtx)
	if err != nil {
		return nil, err
	}
	for idx := frameEndIdx - 1; idx >= frameStartIdx; idx-- {
		if skipped, err := wfr.IsRowSkipped(ctx, idx); err != nil {
			return nil, err
		} else if !skipped {
			row, err := wfr.Rows.GetRow(ctx, idx)
			if err != nil {
				return nil, err
			}
			return row.GetDatum(int(wfr.ArgsIdxs[0]))
		}
	}
	// All rows are skipped from the frame, so it is empty, and we return NULL.
	return tree.DNull, nil
}

// Reset implements eval.WindowFunc interface.
func (lastValueWindow) Reset(context.Context) {}

func (lastValueWindow) Close(context.Context, *eval.Context) {}

// nthValueWindow returns value evaluated at the row that is the nth row of the window frame
// (counting from 1). Returns null if no such row.
type nthValueWindow struct{}

func newNthValueWindow([]*types.T, *eval.Context) eval.WindowFunc {
	return &nthValueWindow{}
}

// ErrInvalidArgumentForNthValue should be thrown when the nth_value window
// function is given a value of 'n' less than zero.
var ErrInvalidArgumentForNthValue = pgerror.Newf(
	pgcode.InvalidParameterValue, "argument of nth_value() must be greater than zero")

func (nthValueWindow) Compute(
	ctx context.Context, evalCtx *eval.Context, wfr *eval.WindowFrameRun,
) (tree.Datum, error) {
	args, err := wfr.Args(ctx)
	if err != nil {
		return nil, err
	}
	arg := args[1]
	if arg == tree.DNull {
		return tree.DNull, nil
	}

	nth := int(tree.MustBeDInt(arg))
	if nth <= 0 {
		return nil, ErrInvalidArgumentForNthValue
	}

	frameStartIdx, err := wfr.FrameStartIdx(ctx, evalCtx)
	if err != nil {
		return nil, err
	}
	frameEndIdx, err := wfr.FrameEndIdx(ctx, evalCtx)
	if err != nil {
		return nil, err
	}
	if nth > frameEndIdx-frameStartIdx {
		// The requested index is definitely outside of the window frame, so we
		// return NULL.
		return tree.DNull, nil
	}
	var idx int
	// Note that we do not need to check whether a filter is present because
	// filters are only supported for aggregate functions.
	if wfr.Frame.DefaultFrameExclusion() {
		// We subtract 1 because nth is counting from 1.
		idx = frameStartIdx + nth - 1
	} else {
		ith := 0
		for idx = frameStartIdx; idx < frameEndIdx; idx++ {
			if skipped, err := wfr.IsRowSkipped(ctx, idx); err != nil {
				return nil, err
			} else if !skipped {
				ith++
				if ith == nth {
					// idx now points at the desired row.
					break
				}
			}
		}
		if idx == frameEndIdx {
			// The requested index is outside of the window frame, so we return NULL.
			return tree.DNull, nil
		}
	}
	row, err := wfr.Rows.GetRow(ctx, idx)
	if err != nil {
		return nil, err
	}
	return row.GetDatum(int(wfr.ArgsIdxs[0]))
}

// Reset implements eval.WindowFunc interface.
func (nthValueWindow) Reset(context.Context) {}

func (nthValueWindow) Close(context.Context, *eval.Context) {}
