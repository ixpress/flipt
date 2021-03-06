package db

import (
	"context"
	"database/sql"

	sq "github.com/Masterminds/squirrel"
	flipt "github.com/markphelps/flipt/rpc"
	"github.com/markphelps/flipt/storage"
	"github.com/sirupsen/logrus"
)

var _ storage.EvaluationStore = &EvaluationStore{}

type optionalConstraint struct {
	ID       sql.NullString
	Type     sql.NullInt64
	Property sql.NullString
	Operator sql.NullString
	Value    sql.NullString
}

// EvaluationStore is a SQL EvaluationStore
type EvaluationStore struct {
	logger  logrus.FieldLogger
	builder sq.StatementBuilderType
}

// NewEvaluationStore creates an EvaluationStore
func NewEvaluationStore(logger logrus.FieldLogger, builder sq.StatementBuilderType) *EvaluationStore {
	return &EvaluationStore{
		logger:  logger,
		builder: builder,
	}
}

func (s *EvaluationStore) GetEvaluationRules(ctx context.Context, flagKey string) ([]*storage.EvaluationRule, error) {
	s.logger.WithField("flagKey", flagKey).Debug("get evaluation rules")

	// get all rules for flag with their constraints if any
	rows, err := s.builder.Select("r.id, r.flag_key, r.segment_key, s.match_type, r.rank, c.id, c.type, c.property, c.operator, c.value").
		From("rules r").
		Join("segments s on (r.segment_key = s.key)").
		LeftJoin("constraints c ON (s.key = c.segment_key)").
		Where(sq.Eq{"r.flag_key": flagKey}).
		OrderBy("r.rank ASC").
		GroupBy("r.id, c.id, s.match_type").
		QueryContext(ctx)
	if err != nil {
		return nil, err
	}

	defer func() {
		if cerr := rows.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	var (
		seenRules = make(map[string]*storage.EvaluationRule)
		rules     = []*storage.EvaluationRule{}
	)

	for rows.Next() {
		var (
			tempRule           storage.EvaluationRule
			optionalConstraint optionalConstraint
		)

		if err := rows.Scan(&tempRule.ID, &tempRule.FlagKey, &tempRule.SegmentKey, &tempRule.SegmentMatchType, &tempRule.Rank, &optionalConstraint.ID, &optionalConstraint.Type, &optionalConstraint.Property, &optionalConstraint.Operator, &optionalConstraint.Value); err != nil {
			return nil, err
		}

		if existingRule, ok := seenRules[tempRule.ID]; ok {
			// current rule we know about
			if optionalConstraint.ID.Valid {
				constraint := storage.EvaluationConstraint{
					ID:       optionalConstraint.ID.String,
					Type:     flipt.ComparisonType(optionalConstraint.Type.Int64),
					Property: optionalConstraint.Property.String,
					Operator: optionalConstraint.Operator.String,
					Value:    optionalConstraint.Value.String,
				}
				existingRule.Constraints = append(existingRule.Constraints, constraint)
			}
		} else {
			// haven't seen this rule before
			newRule := &storage.EvaluationRule{
				ID:               tempRule.ID,
				FlagKey:          tempRule.FlagKey,
				SegmentKey:       tempRule.SegmentKey,
				SegmentMatchType: tempRule.SegmentMatchType,
				Rank:             tempRule.Rank,
			}

			if optionalConstraint.ID.Valid {
				constraint := storage.EvaluationConstraint{
					ID:       optionalConstraint.ID.String,
					Type:     flipt.ComparisonType(optionalConstraint.Type.Int64),
					Property: optionalConstraint.Property.String,
					Operator: optionalConstraint.Operator.String,
					Value:    optionalConstraint.Value.String,
				}
				newRule.Constraints = append(newRule.Constraints, constraint)
			}

			seenRules[newRule.ID] = newRule
			rules = append(rules, newRule)
		}
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	s.logger.WithField("rules", rules).Debug("get evaluation rules")
	return rules, nil
}

func (s *EvaluationStore) GetEvaluationDistributions(ctx context.Context, ruleID string) ([]*storage.EvaluationDistribution, error) {
	s.logger.WithField("ruleID", ruleID).Debug("get evaluation distributions")

	rows, err := s.builder.Select("d.id", "d.rule_id", "d.variant_id", "d.rollout", "v.key").
		From("distributions d").
		Join("variants v ON (d.variant_id = v.id)").
		Where(sq.Eq{"d.rule_id": ruleID}).
		QueryContext(ctx)
	if err != nil {
		return nil, err
	}

	defer func() {
		if cerr := rows.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	var distributions []*storage.EvaluationDistribution

	for rows.Next() {
		var d storage.EvaluationDistribution

		if err := rows.Scan(&d.ID, &d.RuleID, &d.VariantID, &d.Rollout, &d.VariantKey); err != nil {
			return nil, err
		}

		distributions = append(distributions, &d)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	s.logger.WithField("distributions", distributions).Debug("get evaluation distributions")
	return distributions, nil
}
