package service

import (
	"github.com/imrui/xray-pilot/internal/dto"
	"github.com/imrui/xray-pilot/internal/entity"
	"github.com/imrui/xray-pilot/internal/repository"
)

type SyncSummaryService struct {
	nodeRepo *repository.NodeRepository
}

func NewSyncSummaryService() *SyncSummaryService {
	return &SyncSummaryService{nodeRepo: repository.NewNodeRepository()}
}

func (s *SyncSummaryService) GetSyncSummary() (*dto.SyncSummaryResponse, error) {
	counts, err := s.nodeRepo.CountSyncStatuses()
	if err != nil {
		return nil, err
	}

	drifted := int(counts[entity.SyncStatusDrifted])
	failed := int(counts[entity.SyncStatusFailed])
	pending := int(counts[entity.SyncStatusPending])
	total := drifted + failed + pending

	return &dto.SyncSummaryResponse{
		NeedsSync:     total > 0,
		DriftedCount:  drifted,
		FailedCount:   failed,
		PendingCount:  pending,
		TotalAffected: total,
	}, nil
}
