package worker

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/appraisal"
	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/pvp"
)

const (
	defaultPVPEvalQueueBatchSize = 10
	maxPVPEvalErrorLength        = 512
)

var defaultPVPEvalMaxCPCaps = []int{500, 1500, 2500}

type pvpQueueRunner interface {
	ProcessQueue(ctx context.Context, limit int, now time.Time) (int, error)
}

type pvpEvaluationStore interface {
	ClaimPvPEvaluationQueueItems(ctx context.Context, limit int, now time.Time) ([]appraisal.PvPEvaluationQueueItem, error)
	GetResultByID(ctx context.Context, resultID string) (appraisal.Result, bool, error)
	UpsertResultPvPEvaluations(
		ctx context.Context,
		appraisalResultID string,
		evaluations []appraisal.UpsertPvPEvaluationParams,
		now time.Time,
	) error
	MarkPvPEvaluationQueueItemSucceeded(ctx context.Context, queueItemID string, now time.Time) (bool, error)
	MarkPvPEvaluationQueueItemFailed(
		ctx context.Context,
		queueItemID string,
		retryCount int,
		lastError string,
		nextRetryAt *time.Time,
		now time.Time,
	) (bool, error)
}

type maxCPEvaluator interface {
	EvaluateMaxCP(
		speciesID string,
		ivAttack int,
		ivDefense int,
		ivStamina int,
		maxCP int,
	) (pvp.MaxCPEvaluation, error)
}

type forwardEvolutionGraph interface {
	SpeciesIDForCanonicalName(speciesName string) (string, bool)
	SpeciesIDForNormalizedName(normalized string) (string, bool)
	ForwardFamily(speciesID string) []string
}

type pvpEvalRunner struct {
	store     pvpEvaluationStore
	evaluator maxCPEvaluator
	evolution forwardEvolutionGraph
	logger    *slog.Logger
	nowFn     func() time.Time
	maxCPCaps []int
}

func newPVPEvalRunner(
	store pvpEvaluationStore,
	evaluator maxCPEvaluator,
	evolution forwardEvolutionGraph,
	logger *slog.Logger,
) *pvpEvalRunner {
	return &pvpEvalRunner{
		store:     store,
		evaluator: evaluator,
		evolution: evolution,
		logger:    logger,
		nowFn:     time.Now,
		maxCPCaps: append([]int(nil), defaultPVPEvalMaxCPCaps...),
	}
}

func (r *pvpEvalRunner) ProcessQueue(ctx context.Context, limit int, now time.Time) (int, error) {
	if r == nil {
		return 0, nil
	}
	if limit <= 0 {
		return 0, fmt.Errorf("limit must be greater than 0")
	}

	claimTime := normalizePVPEvalNow(now)
	items, err := r.store.ClaimPvPEvaluationQueueItems(ctx, limit, claimTime)
	if err != nil {
		return 0, fmt.Errorf("claim pvp evaluation queue items: %w", err)
	}

	succeeded := 0
	for _, item := range items {
		itemNow := normalizePVPEvalNow(r.nowFn())
		if err := r.processQueueItem(ctx, item, itemNow); err != nil {
			retryCount := item.RetryCount + 1
			nextRetryAt := itemNow.Add(pvpRetryBackoff(retryCount))
			message := truncateString(strings.TrimSpace(err.Error()), maxPVPEvalErrorLength)

			updated, markErr := r.store.MarkPvPEvaluationQueueItemFailed(
				ctx,
				item.ID,
				retryCount,
				message,
				&nextRetryAt,
				itemNow,
			)
			if markErr != nil {
				return succeeded, fmt.Errorf("mark pvp queue item failed: %w", markErr)
			}
			if !updated {
				r.logger.Warn(
					"pvp evaluation queue item failure update lost ownership",
					"queue_item_id", item.ID,
				)
				continue
			}
			r.logger.Warn(
				"pvp evaluation queue item failed",
				"queue_item_id", item.ID,
				"retry_count", retryCount,
				"next_retry_at", nextRetryAt.Format(time.RFC3339Nano),
				"error", message,
			)
			continue
		}

		updated, err := r.store.MarkPvPEvaluationQueueItemSucceeded(ctx, item.ID, itemNow)
		if err != nil {
			return succeeded, fmt.Errorf("mark pvp queue item succeeded: %w", err)
		}
		if !updated {
			r.logger.Warn(
				"pvp evaluation queue item success update lost ownership",
				"queue_item_id", item.ID,
			)
			continue
		}

		succeeded++
	}

	return succeeded, nil
}

func (r *pvpEvalRunner) processQueueItem(
	ctx context.Context,
	item appraisal.PvPEvaluationQueueItem,
	now time.Time,
) error {
	result, found, err := r.store.GetResultByID(ctx, item.AppraisalResultID)
	if err != nil {
		return fmt.Errorf("load appraisal result %q: %w", item.AppraisalResultID, err)
	}
	if !found {
		return fmt.Errorf("appraisal result %q not found", item.AppraisalResultID)
	}

	sourceSpeciesID, err := r.resolveSourceSpeciesID(result.SpeciesName)
	if err != nil {
		return err
	}

	familySpecies := r.evolution.ForwardFamily(sourceSpeciesID)
	if len(familySpecies) == 0 {
		return fmt.Errorf("forward family not found for species %q", sourceSpeciesID)
	}

	evaluations := make([]appraisal.UpsertPvPEvaluationParams, 0, len(familySpecies)*len(r.maxCPCaps))
	for _, evaluatedSpeciesID := range familySpecies {
		for _, maxCP := range r.maxCPCaps {
			evaluation, err := r.evaluator.EvaluateMaxCP(
				evaluatedSpeciesID,
				result.IVAttack,
				result.IVDefense,
				result.IVStamina,
				maxCP,
			)
			if err != nil {
				return fmt.Errorf(
					"evaluate species %q at maxCP %d for result %q: %w",
					evaluatedSpeciesID,
					maxCP,
					result.ID,
					err,
				)
			}

			evaluations = append(evaluations, appraisal.UpsertPvPEvaluationParams{
				MaxCP:              evaluation.MaxCP,
				EvaluatedSpeciesID: evaluation.EvaluatedSpeciesID,
				BestLevel:          evaluation.BestLevel,
				BestCP:             evaluation.BestCP,
				StatProduct:        evaluation.StatProduct,
				RankPosition:       evaluation.Rank,
				Percentage:         evaluation.Percentage,
				CreatedAt:          now,
			})
		}
	}

	if err := r.store.UpsertResultPvPEvaluations(ctx, result.ID, evaluations, now); err != nil {
		return fmt.Errorf("upsert pvp evaluations for result %q: %w", result.ID, err)
	}

	return nil
}

func (r *pvpEvalRunner) resolveSourceSpeciesID(speciesName string) (string, error) {
	canonical := strings.TrimSpace(speciesName)
	if canonical == "" {
		return "", fmt.Errorf("result species name is empty")
	}

	if speciesID, ok := r.evolution.SpeciesIDForCanonicalName(canonical); ok {
		return speciesID, nil
	}

	parsed := appraisal.ParseCandidateFromOCR(canonical)
	if parsed.SpeciesNameNormalized != nil {
		if speciesID, ok := r.evolution.SpeciesIDForNormalizedName(*parsed.SpeciesNameNormalized); ok {
			return speciesID, nil
		}
	}

	normalized := strings.ToLower(strings.Join(strings.Fields(canonical), " "))
	if normalized != "" {
		if speciesID, ok := r.evolution.SpeciesIDForNormalizedName(normalized); ok {
			return speciesID, nil
		}
	}

	return "", fmt.Errorf("result species %q is not present in evolution graph", canonical)
}

func pvpRetryBackoff(retryCount int) time.Duration {
	if retryCount <= 1 {
		return time.Minute
	}

	exponent := retryCount - 1
	if exponent > 5 {
		exponent = 5
	}

	return time.Duration(1<<exponent) * time.Minute
}

func truncateString(value string, maxLen int) string {
	if maxLen <= 0 || len(value) <= maxLen {
		return value
	}
	return value[:maxLen]
}

func normalizePVPEvalNow(now time.Time) time.Time {
	now = now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return now
}
