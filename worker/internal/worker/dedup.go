package worker

import (
	"fmt"
	"sort"
	"strings"

	"github.com/victoroliveirab/vibe-pokemongo-appraisal-app/worker/internal/appraisal"
)

type canonicalAcceptedReading struct {
	Accepted         appraisal.AcceptedResultCandidate
	FrameTimestampMS *int64
}

type pendingVideoReading struct {
	AcceptedOptions  []appraisal.AcceptedResultCandidate
	FrameTimestampMS *int64
}

type indexedCanonicalAcceptedReading struct {
	Index   int
	Reading canonicalAcceptedReading
}

type indexedPendingVideoReading struct {
	Index   int
	Reading pendingVideoReading
}

const adjacentFrameCollapseWindowMS int64 = 900

func deduplicateCanonicalAcceptedReadings(readings []canonicalAcceptedReading) []canonicalAcceptedReading {
	if len(readings) == 0 {
		return nil
	}

	indexed := make([]indexedCanonicalAcceptedReading, 0, len(readings))
	for idx, reading := range readings {
		indexed = append(indexed, indexedCanonicalAcceptedReading{
			Index:   idx,
			Reading: reading,
		})
	}

	sort.SliceStable(indexed, func(i int, j int) bool {
		leftTS := canonicalTimestampSortValue(indexed[i].Reading.FrameTimestampMS)
		rightTS := canonicalTimestampSortValue(indexed[j].Reading.FrameTimestampMS)
		if leftTS != rightTS {
			return leftTS < rightTS
		}
		return indexed[i].Index < indexed[j].Index
	})

	seen := make(map[string]struct{}, len(indexed))
	deduped := make([]canonicalAcceptedReading, 0, len(indexed))
	for _, entry := range indexed {
		key := canonicalDedupKey(entry.Reading)
		if _, exists := seen[key]; exists {
			continue
		}

		seen[key] = struct{}{}
		deduped = append(deduped, entry.Reading)
	}

	return deduped
}

func reduceAcceptedVideoReadings(readings []canonicalAcceptedReading) []canonicalAcceptedReading {
	strictDeduped := deduplicateCanonicalAcceptedReadings(readings)
	if len(strictDeduped) <= 1 {
		return strictDeduped
	}

	lastTimestampByKey := make(map[string]int64, len(strictDeduped))
	filtered := make([]canonicalAcceptedReading, 0, len(strictDeduped))
	for _, reading := range strictDeduped {
		key := adjacentFrameDedupKey(reading)
		timestamp := canonicalTimestampSortValue(reading.FrameTimestampMS)
		if lastTimestamp, ok := lastTimestampByKey[key]; ok {
			if timestamp-lastTimestamp <= adjacentFrameCollapseWindowMS {
				continue
			}
		}

		lastTimestampByKey[key] = timestamp
		filtered = append(filtered, reading)
	}

	return filtered
}

func deduplicatePendingVideoReadings(readings []pendingVideoReading) []pendingVideoReading {
	if len(readings) == 0 {
		return nil
	}

	indexed := make([]indexedPendingVideoReading, 0, len(readings))
	for idx, reading := range readings {
		indexed = append(indexed, indexedPendingVideoReading{
			Index:   idx,
			Reading: reading,
		})
	}

	sort.SliceStable(indexed, func(i int, j int) bool {
		leftTS := canonicalTimestampSortValue(indexed[i].Reading.FrameTimestampMS)
		rightTS := canonicalTimestampSortValue(indexed[j].Reading.FrameTimestampMS)
		if leftTS != rightTS {
			return leftTS < rightTS
		}
		return indexed[i].Index < indexed[j].Index
	})

	seen := make(map[string]struct{}, len(indexed))
	deduped := make([]pendingVideoReading, 0, len(indexed))
	for _, entry := range indexed {
		key := pendingReadingDedupKey(entry.Reading)
		if _, exists := seen[key]; exists {
			continue
		}

		seen[key] = struct{}{}
		deduped = append(deduped, entry.Reading)
	}

	return deduped
}

func canonicalDedupKey(reading canonicalAcceptedReading) string {
	speciesName := strings.ToLower(strings.TrimSpace(reading.Accepted.SpeciesName))

	return fmt.Sprintf(
		"%s|%d|%d|%d|%d|%d|%d",
		speciesName,
		reading.Accepted.CP,
		reading.Accepted.HP,
		0,
		reading.Accepted.IVAttack,
		reading.Accepted.IVDefense,
		reading.Accepted.IVStamina,
	)
}

func adjacentFrameDedupKey(reading canonicalAcceptedReading) string {
	speciesName := strings.ToLower(strings.TrimSpace(reading.Accepted.SpeciesName))
	return fmt.Sprintf(
		"%s|%d|%d|%d",
		speciesName,
		reading.Accepted.CP,
		reading.Accepted.HP,
		0,
	)
}

func pendingReadingDedupKey(reading pendingVideoReading) string {
	if len(reading.AcceptedOptions) == 0 {
		return ""
	}

	chosen := reading.AcceptedOptions[0]
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf(
		"%d|%d|%d|%d|%d|%d",
		chosen.CP,
		chosen.HP,
		0,
		chosen.IVAttack,
		chosen.IVDefense,
		chosen.IVStamina,
	))

	for idx, option := range reading.AcceptedOptions {
		speciesNormalized := strings.TrimSpace(option.SpeciesNameNormalized)
		if speciesNormalized == "" {
			speciesNormalized = strings.ToLower(strings.TrimSpace(option.SpeciesName))
		}
		builder.WriteString(fmt.Sprintf(
			"|%d:%s:%s:%d",
			idx+1,
			speciesNormalized,
			strings.TrimSpace(option.MatchMode),
			option.MatchDistance,
		))
	}

	return builder.String()
}

func canonicalTimestampSortValue(timestampMS *int64) int64 {
	if timestampMS == nil {
		return int64(1<<63 - 1)
	}
	return *timestampMS
}
