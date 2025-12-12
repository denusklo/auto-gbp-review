package socialmedia

import (
	"log"
	"os"
	"strconv"
	"time"
)

// Scheduler handles periodic synchronization of reviews from social media platforms
type Scheduler struct {
	syncService  *SyncService
	interval     time.Duration
	batchSize    int
	ticker       *time.Ticker
	stopChan     chan struct{}
	isRunning    bool
}

// NewScheduler creates a new scheduler with the sync service
func NewScheduler(syncService *SyncService) *Scheduler {
	// Get interval from environment or use default (6 hours)
	intervalHours := 6
	if envInterval := os.Getenv("SYNC_INTERVAL_HOURS"); envInterval != "" {
		if parsed, err := strconv.Atoi(envInterval); err == nil {
			intervalHours = parsed
		}
	}

	// Get batch size from environment or use default (10)
	batchSize := 10
	if envBatch := os.Getenv("SYNC_BATCH_SIZE"); envBatch != "" {
		if parsed, err := strconv.Atoi(envBatch); err == nil {
			batchSize = parsed
		}
	}

	return &Scheduler{
		syncService: syncService,
		interval:    time.Duration(intervalHours) * time.Hour,
		batchSize:   batchSize,
		stopChan:    make(chan struct{}),
		isRunning:   false,
	}
}

// Start begins the scheduled synchronization
func (s *Scheduler) Start() {
	if s.isRunning {
		log.Println("[Scheduler] Already running")
		return
	}

	s.isRunning = true
	s.ticker = time.NewTicker(s.interval)

	log.Printf("[Scheduler] Starting with interval: %v, batch size: %d\n", s.interval, s.batchSize)

	// Run initial sync after a short delay
	go func() {
		time.Sleep(30 * time.Second)
		s.runSync()
	}()

	// Run periodic syncs
	go func() {
		for {
			select {
			case <-s.ticker.C:
				s.runSync()
			case <-s.stopChan:
				s.ticker.Stop()
				log.Println("[Scheduler] Stopped")
				return
			}
		}
	}()
}

// Stop stops the scheduled synchronization
func (s *Scheduler) Stop() {
	if !s.isRunning {
		return
	}

	s.isRunning = false
	close(s.stopChan)
}

// runSync executes the synchronization process
func (s *Scheduler) runSync() {
	log.Println("[Scheduler] Starting scheduled sync...")

	startTime := time.Now()

	// Get all active connections
	connections, err := s.syncService.db.GetActiveConnections()
	if err != nil {
		log.Printf("[Scheduler] Error getting active connections: %v\n", err)
		return
	}

	if len(connections) == 0 {
		log.Println("[Scheduler] No active connections to sync")
		return
	}

	log.Printf("[Scheduler] Found %d active connection(s)\n", len(connections))

	// Sync connections in batches
	successCount := 0
	failCount := 0

	for i := 0; i < len(connections); i += s.batchSize {
		end := i + s.batchSize
		if end > len(connections) {
			end = len(connections)
		}

		batch := connections[i:end]
		log.Printf("[Scheduler] Processing batch %d-%d of %d\n", i+1, end, len(connections))

		// Process batch concurrently
		results := make(chan SyncResult, len(batch))

		for _, conn := range batch {
			go func(connection *APIConnection) {
				result := SyncResult{ConnectionID: connection.ID}

				// Skip if currently syncing
				if connection.SyncStatus == SyncStatusSyncing {
					result.Skipped = true
					results <- result
					return
				}

				stats, err := s.syncService.SyncConnection(connection.ID, SyncTypeScheduled)
				if err != nil {
					result.Error = err
					log.Printf("[Scheduler] Error syncing connection %d (%s): %v\n",
						connection.ID, connection.Platform, err)
				} else {
					result.Stats = stats
					log.Printf("[Scheduler] Successfully synced connection %d (%s): fetched=%d, added=%d, updated=%d\n",
						connection.ID, connection.Platform, stats.TotalFetched, stats.TotalAdded, stats.TotalUpdated)
				}

				results <- result
			}(conn)
		}

		// Collect results
		for j := 0; j < len(batch); j++ {
			result := <-results
			if result.Skipped {
				continue
			}
			if result.Error != nil {
				failCount++
			} else {
				successCount++
			}
		}

		// Rate limiting: wait between batches
		if end < len(connections) {
			time.Sleep(5 * time.Second)
		}
	}

	duration := time.Since(startTime)
	log.Printf("[Scheduler] Sync completed in %v: %d succeeded, %d failed\n",
		duration, successCount, failCount)
}

// SyncResult holds the result of a sync operation
type SyncResult struct {
	ConnectionID int
	Stats        *SyncStats
	Error        error
	Skipped      bool
}

// RunManualSync triggers a manual sync for a specific connection
func (s *Scheduler) RunManualSync(connectionID int) (*SyncStats, error) {
	log.Printf("[Scheduler] Running manual sync for connection %d\n", connectionID)
	return s.syncService.SyncConnection(connectionID, SyncTypeManual)
}

// GetStatus returns the current status of the scheduler
func (s *Scheduler) GetStatus() map[string]interface{} {
	return map[string]interface{}{
		"is_running":   s.isRunning,
		"interval":     s.interval.String(),
		"batch_size":   s.batchSize,
		"next_run_in":  s.getTimeUntilNextRun(),
	}
}

// getTimeUntilNextRun calculates time until next scheduled run
func (s *Scheduler) getTimeUntilNextRun() string {
	if !s.isRunning || s.ticker == nil {
		return "N/A"
	}

	// This is an approximation since we can't directly get the next tick time
	return s.interval.String()
}

// SyncStats helper methods

// HasErrors returns true if there were any errors during sync
func (s *SyncStats) HasErrors() bool {
	return len(s.Errors) > 0
}

// GetErrorMessages returns all error messages as strings
func (s *SyncStats) GetErrorMessages() []string {
	messages := make([]string, len(s.Errors))
	for i, err := range s.Errors {
		messages[i] = err.Error()
	}
	return messages
}

// Summary returns a human-readable summary of the sync
func (s *SyncStats) Summary() string {
	if s.HasErrors() {
		return "Completed with errors"
	}
	if s.TotalFetched == 0 {
		return "No new reviews found"
	}
	return "Completed successfully"
}
