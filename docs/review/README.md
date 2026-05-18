# Review Findings Index

Priority is the fix-order bucket derived from severity and current status:
`P0` is Critical, `P1` is High, `P2` is Medium, and `P3` is Low, stale,
partial, dismissed, or design-only/static work. Severity remains the finding's
impact classification.

| Priority | Severity | Category | Finding | Review status | Reproducer test status |
|---|---|---|---|---|---|
| P0 | Critical | Concurrency | [gRPC Stream Send Data Race](grpc-stream-send-race.md) | Confirmed | Not added. Needs fake stream with concurrent send timing. |
| P0 | Critical | Concurrency | [Watch Distribute Deadlock](watch-distribute-deadlock.md) | Confirmed | Added: `TestDistributeBlocksWithFullWatcherInbox` in [internal/watch/repro_test.go](../../internal/watch/repro_test.go). |
| P0 | Critical | Data ownership | [Watch Response Shared Pointer Corruption](watch-response-shared-pointer.md) | Confirmed | Added: `TestDistributeReusesEventPointerAcrossQueuedResponses` in [internal/watch/repro_test.go](../../internal/watch/repro_test.go). |
| P1 | High | Cluster correctness | [Split-Brain Detection Gap](split-brain-detection-gap.md) | Design hazard | Not added. Needs multi-node/fencing integration harness. |
| P1 | High | Cluster correctness | [ApplyClusterState Not Atomic](apply-cluster-state-not-atomic.md) | Confirmed | Not added. Needs controlled interleaving between state transitions. |
| P1 | High | Cluster correctness | [previousPrimary Data Race](previous-primary-data-race.md) | Confirmed | Not added. Race-detector repro would need deterministic concurrent election/preflight harness. |
| P1 | High | Availability | [Elector Bootstrap Failure Permanently Prevents Primary Election](elector-bootstrap-stuck.md) | Confirmed | Added: `TestOnAcquireLeadershipReturnsNilAfterBootstrapFailure` in [internal/elector/bootstrap_repro_test.go](../../internal/elector/bootstrap_repro_test.go). |
| P1 | High | Concurrency | [Elector Lifecycle Race - No WaitGroup Between Epochs](elector-lifecycle-no-waitgroup.md) | Confirmed | Not added. Needs lifecycle harness that can observe stale goroutines across leadership epochs. |
| P1 | High | Lifecycle | [Recovery Goroutine Loops Forever With No Cancellation](recovery-goroutine-leak.md) | Confirmed | Not added. Needs recoverer owner/shutdown harness. |
| P1 | High | Request correctness | ["List All Keys" Range Request Produces Invalid SQL](list-all-keys-sql-error.md) | Confirmed | Added: `TestRangeAllKeysReproducesInvalidSQL` in [internal/commonapi/range_repro_test.go](../../internal/commonapi/range_repro_test.go). |
| P1 | High | Resource bounds | [SQLite Connection Pool Unbounded](sqlite-pool-unbounded.md) | Confirmed | Added: `TestConnectLeavesSQLitePoolUnbounded` in [internal/localdb/repro_test.go](../../internal/localdb/repro_test.go). |
| P2 | Medium | Cluster correctness | [preflightCancel Accessed Under Inconsistent Mutexes](preflight-cancel-inconsistent-locks.md) | Confirmed | Not added. Race-detector repro would need controlled concurrent preflight cancellation. |
| P2 | Medium | Cluster correctness | [TOCTOU Between requireActivePrimary and leaderTxnMutex](toctou-require-active-primary.md) | Confirmed | Not added. Needs controlled primary-drain interleaving. |
| P2 | Medium | Error contract | [ApplyTxn Silently Swallows All LeaderTxn Errors](apply-txn-swallows-errors.md) | Confirmed | Added: `TestApplyTxnReproducesSwallowedLeaderTxnError` in [internal/clientapi/apply_txn_repro_test.go](../../internal/clientapi/apply_txn_repro_test.go). |
| P2 | Medium | Data ownership | [ReplicateRecord and InsertRecord Mutate Caller's Protobuf](replicate-insert-proto-mutation.md) | Confirmed | Added: `TestInsertRecordMutatesInputOnFailedCompare` and `TestReplicateRecordMutatesInputOnDuplicateConflict` in [internal/localdb/repro_test.go](../../internal/localdb/repro_test.go). |
| P2 | Medium | Data ownership | [Range Key Append Mutates Caller's Backing Array](range-key-append-mutation.md) | Confirmed | Added: `TestRangeMutatesKeyBackingArray` in [internal/commonapi/range_repro_test.go](../../internal/commonapi/range_repro_test.go). |
| P2 | Medium | Data ownership | [Watch Key Append Data Race Under Read Lock](watch-key-append-race.md) | Confirmed | Added: `TestIsInRangeMutatesRangeKeyBackingArray` in [internal/watch/repro_test.go](../../internal/watch/repro_test.go). |
| P2 | Medium | Request correctness | [Panic on Empty Key in Range QueryParts](panic-empty-key-range.md) | Confirmed | Added: `TestRangeEmptyKeyReproducesPanic` in [internal/commonapi/range_repro_test.go](../../internal/commonapi/range_repro_test.go). |
| P2 | Medium | Request correctness | [Panic on Empty rangeKey in Watch isInRange](panic-empty-key-watch.md) | Confirmed | Added: `TestIsInRangeEmptyRangeKeyReproducesPanic` in [internal/watch/repro_test.go](../../internal/watch/repro_test.go). |
| P2 | Medium | Concurrency | [degradeSelf TOCTOU Across Parallel Goroutines](degrade-self-toctou.md) | Confirmed | Not added. Needs deterministic parallel degradation and health transition harness. |
| P2 | Medium | Concurrency | [SetWatchAdmissionFloor TOCTOU Between Store and Validation](watch-admission-floor-toctou.md) | Confirmed | Not added. Needs controlled watch registration during floor update. |
| P2 | Medium | Concurrency | [Follower Compaction Goroutine Has No Context or Shutdown Coordination](follower-compaction-goroutine.md) | Confirmed | Added: `TestFollowerStopDoesNotWaitForInFlightCompaction` in [internal/replication/follower_compaction_repro_test.go](../../internal/replication/follower_compaction_repro_test.go). |
| P2 | Medium | Concurrency | [Snapshot Worker Stop Does Not Wait for In-Flight Work](snapshot-worker-stop-no-wait.md) | Confirmed | Added: `TestWorkerStopDoesNotWaitForInFlightSnapshot` in [internal/snapshot/worker_repro_test.go](../../internal/snapshot/worker_repro_test.go). |
| P2 | Medium | Lifecycle | [Fire-and-Forget Flush Goroutine on Quorum Rollback](quorum-rollback-flush-goroutine.md) | Confirmed | Not added. Needs quorum rollback path with observable in-flight flush. |
| P2 | Medium | Lifecycle | [Health Server Uses Hard Close Instead of Graceful Shutdown](health-server-hard-close.md) | Confirmed | Not added. Needs live HTTP request drain/close harness. |
| P2 | Medium | Resource bounds | [HTTP Servers Missing All Timeouts](http-servers-missing-timeouts.md) | Confirmed | Added: `TestHTTPServerTimeoutsAreZero` in [internal/healthserver/server_repro_test.go](../../internal/healthserver/server_repro_test.go). |
| P2 | Medium | Resource bounds | [GCS/S3 Client Initialization Blocks Indefinitely](storage-init-no-timeout.md) | Confirmed | Not added. Needs fake blocking cloud client initialization path. |
| P2 | Medium | Error contract | [Follow Stream Send-Loop Errors Silently Dropped](follow-send-loop-error-dropped.md) | Confirmed | Not added. Needs fake stream send-loop failure harness. |
| P2 | Medium | Observability | [Compare-Failure Counted as Success in Write Metrics](compare-failure-metrics-miscount.md) | Confirmed | Added: `TestCompareFailureIncrementsSuccessWriteMetric` in [internal/primary/metrics_repro_test.go](../../internal/primary/metrics_repro_test.go). |
| P2 | Medium | Resource cleanup | [datafile Reader Never Closes zstd Decoder](zstd-decoder-resource-leak.md) | Confirmed | Not added. Needs decoder lifecycle instrumentation. |
| P2 | Medium | API design hazard | [FindRecordsBy Accepts Raw SQL WHERE Clause](find-records-raw-sql-hazard.md) | Confirmed design hazard | Not added. Static API hazard; covered indirectly by the invalid range SQL repro. |
| P2 | Medium | TLS configuration | [TLS 1.3 Cipher Suite Restriction Silently Ineffective](tls13-cipher-suite-ineffective.md) | Confirmed | Not added. Runtime behavior follows Go TLS contract; static/config assertion is sufficient. |
| P3 | High, stale | Resource cleanup | [Temp File Leak in DownloadAndImportFile](temp-file-leak-download.md) | Stale against current code | Added regression: `TestDownloadAndImportFileCleansLargeTempFileOnReaderError` in [internal/datastore/download_test.go](../../internal/datastore/download_test.go). |
| P3 | Low, partial | Process lifecycle | [os.Exit Throughout root.go Bypasses Deferred Cleanups](PARTIAL-os-exit-skips-defers.md) | Partial | Not added. Needs subprocess startup-failure harness to observe skipped defers. |
| P3 | N/A | Dismissed | [s.lastWritePath Data Race](DISMISSED-last-write-path-race.md) | Dismissed false positive | N/A. Current access is under `leaderTxnMutex`. |
| P3 | N/A | Dismissed | [s.server.metrics Data Race](DISMISSED-metrics-field-race.md) | Dismissed false positive | N/A. Current wiring sets metrics before the server goroutine starts. |
