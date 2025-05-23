// Copyright 2021 The Cockroach Authors.
//
// Use of this software is governed by the CockroachDB Software License
// included in the /LICENSE file.

package rules

import (
	"fmt"
	"strings"

	"github.com/cockroachdb/cockroach/pkg/clusterversion"
	"github.com/cockroachdb/cockroach/pkg/sql/schemachanger/rel"
	"github.com/cockroachdb/cockroach/pkg/sql/schemachanger/scpb"
	"github.com/cockroachdb/cockroach/pkg/sql/schemachanger/scplan/internal/scgraph"
	"github.com/cockroachdb/cockroach/pkg/sql/schemachanger/screl"
	"github.com/cockroachdb/cockroach/pkg/sql/sem/catid"
	"github.com/cockroachdb/cockroach/pkg/util/iterutil"
	"github.com/cockroachdb/errors"
)

func join(a, b NodeVars, attr rel.Attr, eqVarName rel.Var) rel.Clause {
	return JoinOn(a, attr, b, attr, eqVarName)
}

var _ = join

// JoinOn joins on two node variable attributes, requiring them to have
// the same value.
func JoinOn(a NodeVars, aAttr rel.Attr, b NodeVars, bAttr rel.Attr, eqVarName rel.Var) rel.Clause {
	return rel.And(
		a.El.AttrEqVar(aAttr, eqVarName),
		b.El.AttrEqVar(bAttr, eqVarName),
	)
}

// FilterElements is used to construct a clause which runs an arbitrary predicate
// // over variables.
func FilterElements(name string, a, b NodeVars, fn interface{}) rel.Clause {
	return rel.Filter(name, a.El, b.El)(fn)
}

// ToPublicOrTransient is used to construct a clause that will require both
// elements to be targeting a public/transient state.
func ToPublicOrTransient(from, to NodeVars) rel.Clause {
	return toPublicOrTransientUntyped(from.Target, to.Target)
}

// StatusesToPublicOrTransient requires that elements have a target of
// ToPublicOrTransient and that the current status is fromStatus, toStatus.
func StatusesToPublicOrTransient(
	from NodeVars, fromStatus scpb.Status, to NodeVars, toStatus scpb.Status,
) rel.Clause {
	return rel.And(
		ToPublicOrTransient(from, to),
		from.CurrentStatus(fromStatus),
		to.CurrentStatus(toStatus),
	)
}

func toAbsent(from, to NodeVars) rel.Clause {
	return toAbsentUntyped(from.Target, to.Target)
}

// TransientPublicPrecedesInitialPublic requires that the transient node
// is ToTransientPublic and absent before. The other node is targeting ToPublic.
func TransientPublicPrecedesInitialPublic(transientNode, otherNode NodeVars) rel.Clause {
	return rel.And(
		toPublicToTransientPublicUntyped(otherNode.Target, transientNode.Target),
		transientNode.CurrentStatus(scpb.Status_ABSENT),
	)
}

// TransientPublicPrecedesInitialDrop requires that the transient node
// is ToTransientPublic and absent before. The other node is in ToDrop or
// ToTransient.
func TransientPublicPrecedesInitialDrop(transientNode, otherNode NodeVars) rel.Clause {
	return rel.And(
		toDropToTransientPublicUntyped(otherNode.Target, transientNode.Target),
		transientNode.CurrentStatus(scpb.Status_ABSENT),
	)
}

// PublicTerminalPrecedesTransientPublic requires the transient node is
// ToTransientPublic and reached its final state only. The other node
// is also init its terminal add state.
func PublicTerminalPrecedesTransientPublic(otherNode, transientNode NodeVars) rel.Clause {
	return rel.And(
		toPublicToTransientPublicUntyped(otherNode.Target, transientNode.Target),
		transientNode.CurrentStatus(scpb.Status_TRANSIENT_PUBLIC),
		otherNode.CurrentStatus(scpb.Status_PUBLIC),
	)
}

// DropTerminalPrecedesTransientPublic requires the transient node is
// ToTransientPublic and reached its final state only. The other node
// is also init its terminal drop state.
func DropTerminalPrecedesTransientPublic(otherNode, transientNode NodeVars) rel.Clause {
	return rel.And(
		toDropToTransientPublicUntyped(otherNode.Target, transientNode.Target),
		transientNode.CurrentStatus(scpb.Status_TRANSIENT_PUBLIC),
		otherNode.CurrentStatus(scpb.Status_ABSENT),
	)
}

// StatusesToAbsent requires that elements have a target of
// toAbsent and that the current status is fromStatus/toStatus.
func StatusesToAbsent(
	from NodeVars, fromStatus scpb.Status, to NodeVars, toStatus scpb.Status,
) rel.Clause {
	return rel.And(
		toAbsent(from, to),
		from.CurrentStatus(fromStatus),
		to.CurrentStatus(toStatus),
	)
}

func transient(from, to NodeVars) rel.Clause {
	return transientUntyped(from.Target, to.Target)
}

// StatusesTransient requires that elements have a target of
// transient and that the current status is fromStatus/toStatus.
func StatusesTransient(
	from NodeVars, fromStatus scpb.Status, to NodeVars, toStatus scpb.Status,
) rel.Clause {
	return rel.And(
		transient(from, to),
		from.CurrentStatus(fromStatus),
		to.CurrentStatus(toStatus),
	)
}

// JoinOnDescID joins elements on descriptor ID.
func JoinOnDescID(a, b NodeVars, descriptorIDVar rel.Var) rel.Clause {
	return JoinOnDescIDUntyped(a.El, b.El, descriptorIDVar)
}

// JoinReferencedDescID joins elements on referenced descriptor ID.
func JoinReferencedDescID(a, b NodeVars, descriptorIDVar rel.Var) rel.Clause {
	return joinReferencedDescIDUntyped(a.El, b.El, descriptorIDVar)
}

// JoinOnColumnID joins elements on column ID.
func JoinOnColumnID(a, b NodeVars, relationIDVar, columnIDVar rel.Var) rel.Clause {
	return joinOnColumnIDUntyped(a.El, b.El, relationIDVar, columnIDVar)
}

func JoinOnColumnName(a, b NodeVars, relationIDVar, columnNameVar rel.Var) rel.Clause {
	return joinOnColumnNameUntyped(a.El, b.El, relationIDVar, columnNameVar)
}

// JoinOnColumnFamilyID joins elements on column ID.
func JoinOnColumnFamilyID(a, b NodeVars, relationIDVar, columnFamilyIDVar rel.Var) rel.Clause {
	return joinOnColumnFamilyIDUntyped(a.El, b.El, relationIDVar, columnFamilyIDVar)
}

// JoinOnIndexID joins elements on index ID.
func JoinOnIndexID(a, b NodeVars, relationIDVar, indexIDVar rel.Var) rel.Clause {
	return joinOnIndexIDUntyped(a.El, b.El, relationIDVar, indexIDVar)
}

// JoinOnConstraintID joins elements on constraint ID.
func JoinOnConstraintID(a, b NodeVars, relationIDVar, constraintID rel.Var) rel.Clause {
	return joinOnConstraintIDUntyped(a.El, b.El, relationIDVar, constraintID)
}

// JoinOnTriggerID joins elements on trigger ID.
func JoinOnTriggerID(a, b NodeVars, relationIDVar, triggerID rel.Var) rel.Clause {
	return joinOnTriggerIDUntyped(a.El, b.El, relationIDVar, triggerID)
}

// JoinOnPolicyID joins elements on policy ID.
func JoinOnPolicyID(a, b NodeVars, relationIDVar, policyID rel.Var) rel.Clause {
	return joinOnPolicyIDUntyped(a.El, b.El, relationIDVar, policyID)
}

// JoinOnPartitionName joins elements on partition name.
func JoinOnPartitionName(
	a, b NodeVars, relationIDVar, indexIDVar, partitionNameVar rel.Var,
) rel.Clause {
	return joinOnPartitionNameUntyped(a.El, b.El, relationIDVar, indexIDVar, partitionNameVar)
}

// ColumnInIndex requires that a column exists within an index.
func ColumnInIndex(
	indexColumn, index NodeVars, relationIDVar, columnIDVar, indexIDVar rel.Var,
) rel.Clause {
	return columnInIndexUntyped(indexColumn.El, index.El, relationIDVar, columnIDVar, indexIDVar)
}

// ColumnInSwappedInPrimaryIndex requires that a column exists within a
// primary index being swapped.
func ColumnInSwappedInPrimaryIndex(
	indexColumn, index NodeVars, relationIDVar, columnIDVar, indexIDVar rel.Var,
) rel.Clause {
	return columnInSwappedInPrimaryIndexUntyped(indexColumn.El, index.El, relationIDVar, columnIDVar, indexIDVar)
}

func ColumnInSourcePrimaryIndex(
	indexColumn, index NodeVars, relationIDVar, columnIDVar, indexIDVar rel.Var,
) rel.Clause {
	return columnInSourcePrimaryIndex(indexColumn.El, index.El, relationIDVar, columnIDVar, indexIDVar)
}

// IsAlterColumnTypeOp checks if the specified column is undergoing a type alteration.
// columnIDVar represents the column ID of the new column in the operation.
// If only the dropped column ID is available, use the alternative function:
// IsDroppedColumnPartOfAlterColumnTypeOp.
func IsAlterColumnTypeOp(tableIDVar, columnIDVar rel.Var) rel.Clauses {
	column := MkNodeVars("column")
	computeExpression := MkNodeVars("compute-expression")
	return rel.Clauses{
		column.Type((*scpb.Column)(nil)),
		computeExpression.Type((*scpb.ColumnComputeExpression)(nil)),
		JoinOnColumnID(column, computeExpression, tableIDVar, columnIDVar),
		computeExpression.El.AttrEq(screl.Usage, scpb.ColumnComputeExpression_ALTER_TYPE_USING),
		column.JoinTargetNode(),
		computeExpression.JoinTargetNode(),
	}
}

// IsDroppedColumnPartOfAlterColumnTypeOp functions similarly to IsAlterColumnTypeOp
// but operates using the dropped column ID. It checks for a specific compute expression
// applied to the new column. This requires joining the old column with the new column
// by matching on the column name.
func IsDroppedColumnPartOfAlterColumnTypeOp(tableIDVar, oldColumnIDVar rel.Var) rel.Clauses {
	oldColumnName := MkNodeVars("old-column-name")
	newColumnName := MkNodeVars("new-column-name")
	computeExpression := MkNodeVars("compute-expression")
	return rel.Clauses{
		oldColumnName.Type((*scpb.ColumnName)(nil)),
		newColumnName.Type((*scpb.ColumnName)(nil)),
		JoinOnColumnName(oldColumnName, newColumnName, tableIDVar, "column-name"),
		oldColumnName.El.AttrEqVar(screl.ColumnID, oldColumnIDVar),
		oldColumnName.TargetStatus(scpb.ToAbsent),
		newColumnName.TargetStatus(scpb.ToPublic),
		computeExpression.Type((*scpb.ColumnComputeExpression)(nil)),
		JoinOnColumnID(newColumnName, computeExpression, tableIDVar, "new-column-id"),
		computeExpression.El.AttrEq(screl.Usage, scpb.ColumnComputeExpression_ALTER_TYPE_USING),
		oldColumnName.JoinTargetNode(),
		newColumnName.JoinTargetNode(),
		computeExpression.JoinTargetNode(),
	}
}

// IsPotentialSecondaryIndexSwap determines if a secondary index recreate is
// occurring because of a primary key alter.
func IsPotentialSecondaryIndexSwap(indexIdVar rel.Var, tableIDVar rel.Var) rel.Clauses {
	oldIndex := MkNodeVars("old-index")
	newIndex := MkNodeVars("new-index")
	// This rule detects secondary indexes recreated during a primary index swap,
	// by doing the following. It will check if the re-create source index
	// and index ID matches up between an old and new index
	return rel.Clauses{
		oldIndex.Type((*scpb.SecondaryIndex)(nil)),
		newIndex.Type((*scpb.SecondaryIndex)(nil)),
		oldIndex.TargetStatus(scpb.ToAbsent),
		newIndex.TargetStatus(scpb.ToPublic, scpb.TransientAbsent),
		JoinOnDescID(oldIndex, newIndex, tableIDVar),
		newIndex.El.AttrEqVar(screl.IndexID, indexIdVar),
		JoinOn(oldIndex,
			screl.IndexID,
			newIndex,
			screl.RecreateSourceIndexID,
			"old-index-id"),
		oldIndex.JoinTargetNode(),
		newIndex.JoinTargetNode(),
	}
}

var (
	toPublicOrTransientUntyped = screl.Schema.Def2(
		"ToPublicOrTransient",
		"target1", "target2",
		func(target1 rel.Var, target2 rel.Var) rel.Clauses {
			return rel.Clauses{
				target1.AttrIn(screl.TargetStatus, scpb.Status_PUBLIC, scpb.Status_TRANSIENT_ABSENT, scpb.Status_TRANSIENT_PUBLIC),
				target2.AttrIn(screl.TargetStatus, scpb.Status_PUBLIC, scpb.Status_TRANSIENT_ABSENT, scpb.Status_TRANSIENT_PUBLIC),
			}
		})

	toAbsentUntyped = screl.Schema.Def2(
		"toAbsent",
		"target1", "target2",
		func(target1 rel.Var, target2 rel.Var) rel.Clauses {
			return rel.Clauses{
				target1.AttrEq(screl.TargetStatus, scpb.Status_ABSENT),
				target2.AttrEq(screl.TargetStatus, scpb.Status_ABSENT),
			}
		})

	// toDropToTransientPublicUntyped makes sure target1 is targeting
	// DROP/TRANSIENT_DROP and target2 is targeting TRANSIENT_PUBLIC.
	toDropToTransientPublicUntyped = screl.Schema.Def2(
		"toDropToTransientPublicUntyped",
		"target1", "target2",
		func(target1 rel.Var, target2 rel.Var) rel.Clauses {
			return rel.Clauses{
				target1.AttrIn(screl.TargetStatus, scpb.Status_ABSENT, scpb.Status_TRANSIENT_ABSENT),
				target2.AttrEq(screl.TargetStatus, scpb.Status_TRANSIENT_PUBLIC),
			}
		})

	// toPublicToTransientPublicUntyped makes sure target1 is targeting
	// PUBLIC and target2 is targeting TRANSIENT_PUBLIC.
	toPublicToTransientPublicUntyped = screl.Schema.Def2(
		"toPublicToTransientPublicUntyped",
		"target1", "target2",
		func(target1 rel.Var, target2 rel.Var) rel.Clauses {
			return rel.Clauses{
				target1.AttrIn(screl.TargetStatus, scpb.Status_PUBLIC),
				target2.AttrEq(screl.TargetStatus, scpb.Status_TRANSIENT_PUBLIC),
			}
		})

	transientUntyped = screl.Schema.Def2(
		"transient",
		"target1", "target2",
		func(target1 rel.Var, target2 rel.Var) rel.Clauses {
			return rel.Clauses{
				target1.AttrEq(screl.TargetStatus, scpb.Status_TRANSIENT_ABSENT),
				target2.AttrEq(screl.TargetStatus, scpb.Status_TRANSIENT_ABSENT),
			}
		})

	joinReferencedDescIDUntyped = screl.Schema.Def3(
		"joinReferencedDescID", "referrer", "referenced", "id", func(
			referrer, referenced, id rel.Var,
		) rel.Clauses {
			return rel.Clauses{
				referrer.AttrEqVar(screl.ReferencedDescID, id),
				referenced.AttrEqVar(screl.DescID, id),
			}
		})

	// JoinOnDescIDUntyped joins on descriptor ID, in an unsafe non-type safe
	// manner.
	JoinOnDescIDUntyped = screl.Schema.Def3(
		"joinOnDescID", "a", "b", "id", func(
			a, b, id rel.Var,
		) rel.Clauses {
			return rel.Clauses{
				id.Entities(screl.DescID, a, b),
			}
		})
	joinOnIndexIDUntyped = screl.Schema.Def4(
		"joinOnIndexID", "a", "b", "desc-id", "index-id", func(
			a, b, descID, indexID rel.Var,
		) rel.Clauses {
			return rel.Clauses{
				JoinOnDescIDUntyped(a, b, descID),
				indexID.Entities(screl.IndexID, a, b),
			}
		},
	)
	joinOnColumnIDUntyped = screl.Schema.Def4(
		"joinOnColumnID", "a", "b", "desc-id", "col-id", func(
			a, b, descID, colID rel.Var,
		) rel.Clauses {
			return rel.Clauses{
				JoinOnDescIDUntyped(a, b, descID),
				colID.Entities(screl.ColumnID, a, b),
			}
		},
	)

	joinOnColumnNameUntyped = screl.Schema.Def4(
		"joinOnColumnName", "a", "b", "desc-id", "column-name", func(
			a, b, descID, columnName rel.Var,
		) rel.Clauses {
			return rel.Clauses{
				JoinOnDescIDUntyped(a, b, descID),
				columnName.Entities(screl.Name, a, b),
			}
		},
	)

	joinOnColumnFamilyIDUntyped = screl.Schema.Def4(
		"joinOnColumnFamilyID", "a", "b", "desc-id", "family-id", func(
			a, b, descID, familyID rel.Var,
		) rel.Clauses {
			return rel.Clauses{
				JoinOnDescIDUntyped(a, b, descID),
				familyID.Entities(screl.ColumnFamilyID, a, b),
			}
		},
	)
	joinOnConstraintIDUntyped = screl.Schema.Def4(
		"joinOnConstraintID", "a", "b", "desc-id", "constraint-id", func(
			a, b, descID, constraintID rel.Var,
		) rel.Clauses {
			return rel.Clauses{
				JoinOnDescIDUntyped(a, b, descID),
				constraintID.Entities(screl.ConstraintID, a, b),
			}
		},
	)
	joinOnTriggerIDUntyped = screl.Schema.Def4(
		"joinOnTriggerID", "a", "b", "desc-id", "trigger-id", func(
			a, b, descID, triggerID rel.Var,
		) rel.Clauses {
			return rel.Clauses{
				JoinOnDescIDUntyped(a, b, descID),
				triggerID.Entities(screl.TriggerID, a, b),
			}
		},
	)
	joinOnPolicyIDUntyped = screl.Schema.Def4(
		"joinOnPolicyID", "a", "b", "desc-id", "policy-id", func(
			a, b, descID, policyID rel.Var,
		) rel.Clauses {
			return rel.Clauses{
				JoinOnDescIDUntyped(a, b, descID),
				policyID.Entities(screl.PolicyID, a, b),
			}
		},
	)
	joinOnPartitionNameUntyped = screl.Schema.Def5(
		"joinOnPartitionName", "a", "b", "desc-id", "index-id", "partition-name", func(
			a, b, descID, indexID, partitionName rel.Var,
		) rel.Clauses {
			return rel.Clauses{
				JoinOnDescIDUntyped(a, b, descID),
				indexID.Entities(screl.IndexID, a, b),
				partitionName.Entities(screl.PartitionName, a, b),
			}
		},
	)

	columnInIndexUntyped = screl.Schema.Def5(
		"ColumnInIndex",
		"index-column", "index", "table-id", "column-id", "index-id", func(
			indexColumn, index, tableID, columnID, indexID rel.Var,
		) rel.Clauses {
			return rel.Clauses{
				indexColumn.Type((*scpb.IndexColumn)(nil)),
				indexColumn.AttrEqVar(screl.DescID, rel.Blank),
				indexColumn.AttrEqVar(screl.ColumnID, columnID),
				index.AttrEqVar(screl.IndexID, indexID),
				joinOnIndexIDUntyped(index, indexColumn, tableID, indexID),
			}
		})

	sourceIndexIsSetUntyped = screl.Schema.Def1("sourceIndexIsSet", "index", func(
		index rel.Var,
	) rel.Clauses {
		return rel.Clauses{
			index.AttrNeq(screl.SourceIndexID, catid.IndexID(0)),
		}
	})

	columnInSwappedInPrimaryIndexUntyped = screl.Schema.Def5(
		"ColumnInSwappedInPrimaryIndex",
		"index-column", "index", "table-id", "column-id", "index-id", func(
			indexColumn, index, tableID, columnID, indexID rel.Var,
		) rel.Clauses {
			return rel.Clauses{
				columnInIndexUntyped(
					indexColumn, index, tableID, columnID, indexID,
				),
				sourceIndexIsSetUntyped(index),
			}
		})

	columnInSourcePrimaryIndex = screl.Schema.Def5(
		"ColumnInSourcePrimaryIndex",
		"index-column", "index", "table-id", "column-id", "index-id", func(
			indexColumn, index, tableID, columnID, indexID rel.Var,
		) rel.Clauses {
			return rel.Clauses{
				indexColumn.Type((*scpb.IndexColumn)(nil)),
				indexColumn.AttrEqVar(screl.DescID, tableID),
				indexColumn.AttrEqVar(screl.ColumnID, columnID),
				indexColumn.AttrEqVar(screl.IndexID, indexID),
				index.AttrEqVar(screl.SourceIndexID, indexID),
				JoinOnDescIDUntyped(index, indexColumn, tableID),
			}
		})

	// IsNotPotentialSecondaryIndexSwap determines if no secondary index recreation
	// is happening because of a primary key alter.
	IsNotPotentialSecondaryIndexSwap = screl.Schema.DefNotJoin2("no secondary index swap is on going",
		"table-id", "index-id", func(a, b rel.Var) rel.Clauses {
			return IsPotentialSecondaryIndexSwap(b, a)
		})

	// IsNotAlterColumnTypeOp determines if no column alteration in progress. The column
	// passed in must for a new column.
	IsNotAlterColumnTypeOp = screl.Schema.DefNotJoin2("no column type alteration in progress",
		"table-id", "column-id", func(t, c rel.Var) rel.Clauses {
			return IsAlterColumnTypeOp(t, c)
		})

	IsNotDroppedColumnPartOfAlterColumnTypeOp = screl.Schema.DefNotJoin2("dropped column is not part of column type operation",
		"table-id", "old-column-id", func(t, c rel.Var) rel.Clauses { return IsDroppedColumnPartOfAlterColumnTypeOp(t, c) })
)

// ForEachElementInActiveVersion executes a function for each element supported within
// the current active version.
func ForEachElementInActiveVersion(
	version clusterversion.ClusterVersion, fn func(element scpb.Element) error,
) error {
	return scpb.ForEachElementType(func(e scpb.Element) error {
		if screl.VersionSupportsElementUse(e, version) {
			if err := fn(e); err != nil {
				return iterutil.Map(err)
			}
		}
		return nil
	})
}

type elementTypePredicate = func(e scpb.Element) bool

// Or or's a series of element type predicates.
func Or(predicates ...elementTypePredicate) elementTypePredicate {
	return func(e scpb.Element) bool {
		for _, p := range predicates {
			if p(e) {
				return true
			}
		}
		return false
	}
}

// Not not's a element type predicate.
func Not(predicate elementTypePredicate) elementTypePredicate {
	return func(e scpb.Element) bool {
		return !predicate(e)
	}
}

// RegisterDepRuleForDrop is a convenience function which calls
// RegisterDepRule with the cross-product of (ToAbsent,TransientAbsent)^2 Target
// states, which can't easily be composed.
func RegisterDepRuleForDrop(
	r *Registry,
	ruleName scgraph.RuleName,
	kind scgraph.DepEdgeKind,
	from, to string,
	fromStatus, toStatus scpb.Status,
	fn func(from, to NodeVars) rel.Clauses,
) {

	transientFromStatus, okFrom := scpb.GetTransientEquivalent(fromStatus)
	if !okFrom {
		panic(errors.AssertionFailedf("Invalid 'from' status %s", fromStatus))
	}
	transientToStatus, okTo := scpb.GetTransientEquivalent(toStatus)
	if !okTo {
		panic(errors.AssertionFailedf("Invalid 'from' status %s", toStatus))
	}

	r.RegisterDepRule(ruleName, kind, from, to, func(from, to NodeVars) rel.Clauses {
		return append(
			fn(from, to),
			StatusesToAbsent(from, fromStatus, to, toStatus),
		)
	})

	r.RegisterDepRule(ruleName, kind, from, to, func(from, to NodeVars) rel.Clauses {
		return append(
			fn(from, to),
			StatusesTransient(from, transientFromStatus, to, transientToStatus),
		)
	})

	r.RegisterDepRule(ruleName, kind, from, to, func(from, to NodeVars) rel.Clauses {
		return append(
			fn(from, to),
			from.TargetStatus(scpb.TransientAbsent),
			from.CurrentStatus(transientFromStatus),
			to.TargetStatus(scpb.ToAbsent),
			to.CurrentStatus(toStatus),
		)
	})

	r.RegisterDepRule(ruleName, kind, from, to, func(from, to NodeVars) rel.Clauses {
		return append(
			fn(from, to),
			from.TargetStatus(scpb.ToAbsent),
			from.CurrentStatus(fromStatus),
			to.TargetStatus(scpb.TransientAbsent),
			to.CurrentStatus(transientToStatus),
		)
	})
}

// notJoinOnNodeWithStatusIn is a cache to memoize getNotJoinOnNodeWithStatusIn.
var notJoinOnNodeWithStatusIn = map[string]rel.Rule1{}

// GetNotJoinOnNodeWithStatusIn returns a not-join rule which takes a variable
// corresponding to a target in the graph as input and will exclude that target
// if the graph contains a node with that target in any of the listed status
// values.
func GetNotJoinOnNodeWithStatusIn(statues []scpb.Status) rel.Rule1 {
	makeStatusesStrings := func(statuses []scpb.Status) []string {
		ret := make([]string, len(statuses))
		for i, status := range statuses {
			ret[i] = status.String()
		}
		return ret
	}
	makeStatusesString := func(statuses []scpb.Status) string {
		return strings.Join(makeStatusesStrings(statuses), "_")
	}
	boxStatuses := func(input []scpb.Status) []interface{} {
		ret := make([]interface{}, len(input))
		for i, s := range input {
			ret[i] = s
		}
		return ret
	}
	name := makeStatusesString(statues)
	if got, ok := notJoinOnNodeWithStatusIn[name]; ok {
		return got
	}
	r := screl.Schema.DefNotJoin1(
		fmt.Sprintf("nodeNotExistsWithStatusIn_%s", name),
		"sharedTarget", func(target rel.Var) rel.Clauses {
			n := rel.Var("n")
			return rel.Clauses{
				n.Type((*screl.Node)(nil)),
				n.AttrEqVar(screl.Target, target),
				n.AttrIn(screl.CurrentStatus, boxStatuses(statues)...),
			}
		})
	notJoinOnNodeWithStatusIn[name] = r
	return r
}
