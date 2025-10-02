# Incremental Caching Feature - Project Tracker

> **Quick Reference:** 5-week implementation | Target: 90%+ I/O reduction for NFS scans | Status: Planning

---

## Quick Reference

| Item | Details |
|------|---------|
| **Project Duration** | 5 weeks (October 7 - November 8, 2025) |
| **Target Release** | gdu v5.x (TBD) |
| **Project Lead** | TBD |
| **Implementation Plan** | [docs/incremental-caching-implementation-plan.md](/Users/zoran.vukmirica.889/github/gdu/docs/incremental-caching-implementation-plan.md) |
| **Development Branch** | `feature/incremental-caching` |
| **GitHub Issue** | TBD - Create issue to track overall progress |
| **Weekly Sync** | TBD (suggested: Fridays 2pm) |
| **Demo Schedule** | Week 3 (Phase 3 completion), Week 5 (final) |

---

## 1. Project Overview

### Feature Summary

Add incremental caching to gdu (Go Disk Usage) tool to dramatically reduce I/O operations when scanning large network filesystems (NFS/DDN). The feature will cache directory metadata and only re-scan directories that have changed (based on mtime), reducing I/O by 90%+ for typical daily scans.

### Business Value

**Problem**: Current gdu scans traverse all files/directories every time, generating massive I/O on NFS storage that:
- Takes hours for large volumes (10M+ files)
- Overloads shared storage systems
- Prevents daily disk usage monitoring

**Solution**: Incremental caching with intelligent mtime-based invalidation

**Impact**:
- 80-95% I/O reduction for daily scans (only changed directories rescanned)
- Scan time reduced from hours to minutes
- Enables daily monitoring without storage overload
- Configurable cache expiry and I/O throttling

### Target Use Case

**Primary**: Daily disk usage analysis on large NFS/DDN storage volumes (1M+ files)
- Research labs scanning data storage daily
- IT teams monitoring project directories
- Storage administrators tracking growth trends

**Secondary**: Any scenario with repeated scans of mostly-static directory trees

### Success Criteria

- [ ] **Performance**: 90%+ I/O reduction on subsequent scans (1% daily changes scenario)
- [ ] **Speed**: Subsequent scans complete in <10% of initial scan time
- [ ] **Memory**: <50% memory usage compared to regular in-memory mode
- [ ] **Reliability**: Zero cache corruption or data loss incidents
- [ ] **Compatibility**: All existing gdu features work with incremental mode
- [ ] **Usability**: Positive user feedback on NFS performance improvement
- [ ] **Quality**: >85% unit test coverage, all edge cases handled

---

## 2. Timeline & Milestones

### Overall Timeline: 5 Weeks

```
Week 1          Week 2          Week 3          Week 4          Week 5
[Phase 1]────→  [Phase 2]────→  [Phase 3]────→  [Phase 4]────→  [Phase 5]────→  RELEASE
Core Analyzer   I/O Throttle    Statistics      Edge Cases      Integration

Milestone 1     Milestone 2     Milestone 3     Milestone 4     Milestone 5
Basic caching   Throttling      Stats visible   Production      Documentation
works           works           in CLI/TUI      ready           complete
```

### Critical Milestones

| Milestone | Week | Deliverable | Success Criteria | Owner |
|-----------|------|-------------|------------------|-------|
| **M1: Core Working** | 1 | Basic incremental analyzer | First scan caches, second scan uses cache | TBD |
| **M2: Throttling** | 2 | I/O rate limiting | --max-iops flag limits directory reads | TBD |
| **M3: Observability** | 3 | Cache statistics | --show-cache-stats displays hit rates | TBD |
| **M4: Production Ready** | 4 | Edge case handling | All error scenarios tested and handled | TBD |
| **M5: Integration Complete** | 5 | Full integration + docs | All tests pass, docs complete, ready to merge | TBD |

### Dependencies

```
Phase 1 (Core) ───┬──→ Phase 3 (Statistics)
                  │
                  └──→ Phase 2 (Throttling) ───┐
                                                ├──→ Phase 4 (Edge Cases) ──→ Phase 5 (Integration)
                                                │
                  Phase 3 (Statistics) ─────────┘
```

**Critical Path**: Phase 1 → Phase 4 → Phase 5

**Parallel Work Opportunities**:
- Phase 2 (Throttling) and Phase 3 (Statistics) can be developed in parallel after Phase 1
- Documentation can begin in Week 3 alongside Phase 4

---

## 3. Team Roles & Responsibilities

### Suggested Team Composition

| Role | Responsibilities | Skills Needed | Time Commitment |
|------|-----------------|---------------|-----------------|
| **Software Engineer (Go)** | Core implementation (Phases 1-4) | Go, BadgerDB, concurrency, filesystems | Full-time (4 weeks) |
| **Software Engineer (Go)** | I/O throttling, testing (Phase 2-5) | Go, rate limiting, benchmarking | Full-time (3 weeks) |
| **QA Engineer** | Test strategy, edge case testing | Testing methodologies, Go testing | Half-time (5 weeks) |
| **Technical Writer** | Documentation (Phase 5) | Technical writing, markdown | Half-time (1 week) |
| **Code Reviewer** | Code review at each phase | Go, code quality standards | 5-10 hrs/week |
| **Project Manager** | Coordination, tracking, stakeholder updates | Project management | 10 hrs/week |

### Minimal Team Configuration

If resources are limited, a **single experienced Go engineer** can complete the project in 5-6 weeks with:
- Self-testing and code review via PR process
- Documentation created alongside implementation
- Community testing via pre-release

### Skills Required by Phase

**Phase 1**: Go interfaces, BadgerDB, filesystem operations, mtime handling
**Phase 2**: Rate limiting algorithms, golang.org/x/time/rate, performance testing
**Phase 3**: Statistics collection, thread-safe counters, TUI integration (optional)
**Phase 4**: Error handling, edge case analysis, fault tolerance
**Phase 5**: Integration testing, documentation, performance benchmarking

---

## 4. Deliverables Checklist

### Code Deliverables

#### New Files to Create

- [ ] `pkg/analyze/incremental.go` - Main IncrementalAnalyzer implementation (~500 lines)
- [ ] `pkg/analyze/incremental_storage.go` - Extended BadgerDB operations (~300 lines)
- [ ] `pkg/analyze/incremental_stats.go` - Statistics tracking (~150 lines)
- [ ] `pkg/analyze/throttle.go` - I/O throttling (~100 lines)
- [ ] `pkg/analyze/incremental_test.go` - Unit tests (~600 lines)
- [ ] `pkg/analyze/incremental_bench_test.go` - Benchmarks (~200 lines)
- [ ] `pkg/analyze/incremental_storage_test.go` - Storage tests (~300 lines)
- [ ] `cmd/gdu/app/app_incremental_test.go` - Integration tests (~400 lines)

#### Files to Modify

- [ ] `cmd/gdu/app/app.go` - Add flags, instantiate incremental analyzer
- [ ] `tui/tui.go` - Display cache stats in TUI (optional enhancement)
- [ ] `stdout/stdout.go` - Display cache stats in non-interactive mode

#### Total New Code: ~2,550 lines (implementation) + ~1,300 lines (tests) = ~3,850 lines

### Documentation Deliverables

- [ ] `docs/incremental-caching.md` - Comprehensive user guide (~1000 lines)
  - Overview and how it works
  - Quick start guide
  - Configuration options
  - Best practices
  - Troubleshooting
  - Performance tuning
- [ ] `README.md` - Add incremental caching section
- [ ] `configuration.md` - Document all new flags
- [ ] `CHANGELOG.md` - Update with new feature
- [ ] `docs/incremental-caching-project-tracker.md` - This document

### Testing Deliverables

- [ ] Unit tests with >85% coverage
- [ ] Integration tests covering all flag combinations
- [ ] Benchmark tests comparing with/without cache
- [ ] Edge case tests (8+ scenarios)
- [ ] Performance regression tests (CI)
- [ ] Manual NFS testing documentation

### Configuration Examples

- [ ] Example .gdu.yaml with incremental settings
- [ ] Example cron job for daily NFS scans
- [ ] Example usage patterns documentation

---

## 5. Phase-by-Phase Breakdown

### Phase 1: Core Incremental Analyzer (Week 1)

**Dates**: October 7-11, 2025

#### Phase Objectives

- Implement basic incremental caching functionality
- Cache directory metadata in BadgerDB
- Compare mtime to determine if cache is valid
- Load cached data when directories unchanged
- Re-scan when directories have changed

#### Tasks Breakdown

| Task | Description | Est. (days) | Owner | Status | Completion Date | Notes |
|------|-------------|-------------|-------|--------|----------------|-------|
| 1.1 | Create `IncrementalAnalyzer` struct with initialization | 0.5 | PM (AI Agent) | Complete | 2025-10-02 | ✅ Analyzer struct created with all necessary fields |
| 1.2 | Implement `processDir()` with mtime checking logic | 1.0 | PM (AI Agent) | Complete | 2025-10-02 | ✅ Full mtime validation and cache decision logic |
| 1.3 | Create `IncrementalDirMetadata` schema | 0.5 | PM (AI Agent) | Complete | 2025-10-02 | ✅ Schema with gob encoding support |
| 1.4 | Implement BadgerDB storage operations | 1.0 | PM (AI Agent) | Complete | 2025-10-02 | ✅ Store/Load/Delete operations implemented |
| 1.5 | Implement cache loading and validation | 1.0 | PM (AI Agent) | Complete | 2025-10-02 | ✅ Age and mtime validation integrated |
| 1.6 | Implement `rebuildFromCache()` | 0.5 | PM (AI Agent) | Complete | 2025-10-02 | ✅ Recursive cache reconstruction |
| 1.7 | Add CLI flags (`--incremental`, `--cache-max-age`, etc.) | 0.5 | PM (AI Agent) | Complete | 2025-10-02 | ✅ 5 flags added with YAML support |
| 1.8 | Write unit tests for core functionality | 1.0 | PM (AI Agent) | In Progress | - | Next task |
| 1.9 | Manual testing with sample directories | 0.5 | PM (AI Agent) | Not Started | - | Pending |

**Total Estimated Effort**: 6.5 days

#### Acceptance Criteria

- [ ] Initial scan completes and caches all directory metadata to BadgerDB
- [ ] Second scan with no filesystem changes loads 100% from cache
- [ ] Modified directory (changed mtime) triggers re-scan of that subtree only
- [ ] Unchanged subdirectories within modified tree still use cache
- [ ] `--cache-max-age` flag expires old cache entries correctly
- [ ] `--force-full-scan` ignores cache and updates all entries
- [ ] Unit tests achieve >80% coverage for new code
- [ ] No crashes or panics during testing

#### Dependencies

- None (first phase)

#### Key Files

- `pkg/analyze/incremental.go`
- `pkg/analyze/incremental_storage.go`
- `pkg/analyze/incremental_test.go`
- `cmd/gdu/app/app.go`

---

### Phase 2: I/O Throttling (Week 2)

**Dates**: October 14-18, 2025

#### Phase Objectives

- Add configurable I/O rate limiting
- Protect shared NFS storage from scan overload
- Support both IOPS limits and fixed delays
- Maintain compatibility with parallel/sequential modes

#### Tasks Breakdown

| Task | Description | Est. (days) | Owner | Status |
|------|-------------|-------------|-------|--------|
| 2.1 | Design throttling architecture (token bucket) | 0.5 | TBD | Not Started |
| 2.2 | Implement `IOThrottle` struct using golang.org/x/time/rate | 1.0 | TBD | Not Started |
| 2.3 | Integrate throttling into `processDir()` | 0.5 | TBD | Not Started |
| 2.4 | Add `--max-iops` flag support | 0.5 | TBD | Not Started |
| 2.5 | Add `--io-delay` flag support | 0.5 | TBD | Not Started |
| 2.6 | Write unit tests for throttling | 1.0 | TBD | Not Started |
| 2.7 | Benchmark performance impact of throttling | 1.0 | TBD | Not Started |
| 2.8 | Test with parallel and sequential analyzers | 0.5 | TBD | Not Started |

**Total Estimated Effort**: 5.5 days

#### Acceptance Criteria

- [ ] `--max-iops 1000` limits directory scans to ~1000/second
- [ ] `--io-delay 10ms` adds 10ms delay between each directory scan
- [ ] Throttling works correctly in parallel mode (respects limit across goroutines)
- [ ] Throttling works correctly in sequential mode
- [ ] Performance degrades predictably with throttling enabled
- [ ] Benchmark shows expected throughput limits
- [ ] No deadlocks or race conditions with throttling
- [ ] Unit tests cover all throttling configurations

#### Dependencies

- Phase 1 complete (requires IncrementalAnalyzer)

#### Key Files

- `pkg/analyze/throttle.go`
- `pkg/analyze/incremental.go` (modifications)

---

### Phase 3: Cache Statistics & Management (Week 3)

**Dates**: October 21-25, 2025

#### Phase Objectives

- Provide visibility into cache performance
- Track hit rates, I/O reduction, timing
- Display statistics in CLI and TUI
- Export statistics with JSON output

#### Tasks Breakdown

| Task | Description | Est. (days) | Owner | Status |
|------|-------------|-------------|-------|--------|
| 3.1 | Design `CacheStats` struct with thread-safe counters | 0.5 | TBD | Not Started |
| 3.2 | Implement statistics collection in analyzer | 1.0 | TBD | Not Started |
| 3.3 | Add `--show-cache-stats` flag | 0.5 | TBD | Not Started |
| 3.4 | Implement CLI statistics output formatting | 1.0 | TBD | Not Started |
| 3.5 | Integrate cache stats into TUI (optional) | 1.0 | TBD | Not Started |
| 3.6 | Add cache metadata to JSON export | 0.5 | TBD | Not Started |
| 3.7 | Write unit tests for statistics accuracy | 1.0 | TBD | Not Started |

**Total Estimated Effort**: 5.5 days

#### Acceptance Criteria

- [ ] `--show-cache-stats` displays comprehensive statistics after scan
- [ ] Statistics show: hit rate, I/O reduction %, directories scanned/cached/expired
- [ ] Statistics show: scan time, cache load time, total time
- [ ] Statistics are thread-safe (no race conditions)
- [ ] All statistics calculations are accurate (validated by tests)
- [ ] TUI shows cache indicator or statistics modal (optional)
- [ ] JSON export includes cache metadata
- [ ] Statistics exported match displayed values

#### Dependencies

- Phase 1 complete (requires IncrementalAnalyzer with basic stats hooks)

#### Key Files

- `pkg/analyze/incremental_stats.go`
- `pkg/analyze/incremental.go` (modifications)
- `stdout/stdout.go` (modifications)
- `tui/tui.go` (optional modifications)

---

### Phase 4: Edge Cases & Robustness (Week 4)

**Dates**: October 28 - November 1, 2025

#### Phase Objectives

- Handle all error scenarios gracefully
- Ensure production reliability
- Test fault tolerance
- Document limitations

#### Tasks Breakdown

| Task | Description | Est. (days) | Owner | Status |
|------|-------------|-------------|-------|--------|
| 4.1 | Implement clock skew detection and handling | 0.5 | TBD | Not Started |
| 4.2 | Handle deleted directories/files | 0.5 | TBD | Not Started |
| 4.3 | Handle moved/renamed directories | 0.5 | TBD | Not Started |
| 4.4 | Handle permission changes and access errors | 0.5 | TBD | Not Started |
| 4.5 | Implement cache corruption detection and recovery | 1.0 | TBD | Not Started |
| 4.6 | Handle concurrent modifications during scan | 0.5 | TBD | Not Started |
| 4.7 | Optimize large directory handling (streaming) | 1.0 | TBD | Not Started |
| 4.8 | Handle hardlink changes | 0.5 | TBD | Not Started |
| 4.9 | Write comprehensive edge case tests | 1.5 | TBD | Not Started |
| 4.10 | Stress testing and fault injection | 1.0 | TBD | Not Started |

**Total Estimated Effort**: 8 days (overlaps with Phase 5)

#### Edge Cases to Handle

1. **Clock Skew**: NFS server/client time differences
   - Detect when cached mtime is suspiciously future-dated
   - Use cache timestamp delta for validation

2. **Deleted Directories**: Cached path no longer exists
   - Validate path existence before using cache
   - Remove invalid entries from cache

3. **Moved/Renamed Directories**: Directory relocated
   - Accept as cache miss (expected behavior)
   - Document that renames invalidate cache

4. **Permission Changes**: Lost read access
   - Handle permission errors gracefully
   - Mark as error in results (don't crash)

5. **Concurrent Modifications**: Changes during scan
   - Best-effort consistency (same as current gdu)
   - Document eventual consistency model

6. **Cache Corruption**: BadgerDB corruption
   - Detect on open/read
   - Fall back to full scan automatically
   - Log warning and continue

7. **Large Directories**: Millions of files in one directory
   - Stream file metadata instead of loading all at once
   - Implement pagination for cache reads

8. **Hardlink Changes**: Hardlinks created/removed
   - Detect via item count changes
   - Re-scan if count mismatch

#### Acceptance Criteria

- [ ] All 8 edge cases have unit tests
- [ ] All edge cases handled without crashes
- [ ] Clear error messages for all failure modes
- [ ] Graceful fallback to full scan on cache errors
- [ ] No data loss or corruption in any scenario
- [ ] Performance remains acceptable in worst-case scenarios
- [ ] Error handling documented in code comments
- [ ] Edge case behavior documented in user guide

#### Dependencies

- Phase 1 complete (core implementation to enhance)

#### Key Files

- `pkg/analyze/incremental.go` (extensive modifications)
- `pkg/analyze/incremental_storage.go` (modifications)
- `pkg/analyze/incremental_test.go` (additions)

---

### Phase 5: Integration & Documentation (Week 5)

**Dates**: November 4-8, 2025

#### Phase Objectives

- Ensure compatibility with all existing features
- Complete comprehensive documentation
- Perform final testing and benchmarking
- Prepare for release

#### Tasks Breakdown

| Task | Description | Est. (days) | Owner | Status |
|------|-------------|-------------|-------|--------|
| 5.1 | Test incremental mode with all existing flags | 1.0 | TBD | Not Started |
| 5.2 | Test JSON export/import compatibility | 0.5 | TBD | Not Started |
| 5.3 | Test interactive TUI mode | 0.5 | TBD | Not Started |
| 5.4 | Test non-interactive mode | 0.5 | TBD | Not Started |
| 5.5 | Write `docs/incremental-caching.md` user guide | 1.5 | TBD | Not Started |
| 5.6 | Update `README.md` with incremental section | 0.5 | TBD | Not Started |
| 5.7 | Update `configuration.md` with new flags | 0.5 | TBD | Not Started |
| 5.8 | Create example configurations | 0.5 | TBD | Not Started |
| 5.9 | Perform comprehensive benchmarking | 1.0 | TBD | Not Started |
| 5.10 | Write integration tests for all combinations | 1.0 | TBD | Not Started |
| 5.11 | Final code review and cleanup | 1.0 | TBD | Not Started |
| 5.12 | Update CHANGELOG.md | 0.5 | TBD | Not Started |

**Total Estimated Effort**: 9 days (overlaps with Phase 4)

#### Compatibility Testing Matrix

| Feature | Compatible | Test Status | Notes |
|---------|-----------|-------------|-------|
| `--output-file` (JSON export) | ✅ Yes | [ ] | Export includes cache metadata |
| `--input-file` (JSON import) | ✅ Yes | [ ] | Can import to populate cache |
| `--sequential` | ✅ Yes | [ ] | Works with sequential analyzer |
| `--use-storage` | ⚠️ Exclusive | [ ] | Mutually exclusive flags |
| `--no-cross` | ✅ Yes | [ ] | Cache respects filesystem boundaries |
| `--ignore-dirs` | ✅ Yes | [ ] | Ignored dirs not cached |
| `--follow-symlinks` | ✅ Yes | [ ] | Symlink targets included in cache |
| Interactive TUI | ✅ Yes | [ ] | Full compatibility |
| Non-interactive mode | ✅ Yes | [ ] | Full compatibility |
| `--max-cores` | ✅ Yes | [ ] | Parallel scanning works |
| `--const-gc` | ✅ Yes | [ ] | GC settings respected |

#### Documentation Deliverables

**1. docs/incremental-caching.md** (comprehensive user guide)
- Overview and benefits
- How it works (mtime-based validation)
- Quick start guide
- Configuration options (all flags)
- Best practices
  - Setting cache-max-age for data freshness
  - Using max-iops on shared NFS
  - Running force-full-scan weekly
- Performance tuning
- Troubleshooting common issues
- FAQ

**2. README.md updates**
- Add "Incremental Caching" section
- Quick example usage
- Link to detailed guide

**3. configuration.md updates**
- Document all new flags with examples
- Document YAML configuration

**4. Example configurations**
- Daily NFS scan cron job
- Hourly monitoring with throttling
- Production .gdu.yaml template

#### Acceptance Criteria

- [ ] All existing unit tests pass
- [ ] All integration tests pass
- [ ] New integration tests cover all flag combinations
- [ ] Benchmark shows 90%+ I/O reduction (1% change scenario)
- [ ] Benchmark shows 15x+ speedup on subsequent scans
- [ ] Documentation complete and reviewed
- [ ] Example configurations tested and working
- [ ] No regressions in existing functionality
- [ ] Code coverage >85%
- [ ] All linters pass (golangci-lint)
- [ ] Ready for PR and code review

#### Dependencies

- All previous phases complete

#### Key Files

- All implementation files (review and cleanup)
- `docs/incremental-caching.md` (new)
- `README.md` (modifications)
- `configuration.md` (modifications)
- `cmd/gdu/app/app_incremental_test.go` (new)

---

## 6. Risk Management

### Key Risks & Mitigation

#### Risk 1: mtime Unreliability on NFS

| Attribute | Details |
|-----------|---------|
| **Risk** | NFS mtime may not update immediately after changes, causing stale cache |
| **Likelihood** | Medium |
| **Impact** | High (incorrect results) |
| **Detection** | File changes not reflected in subsequent scans |
| **Mitigation** | - Document mtime limitations in user guide<br>- Add `--no-cache-trust-mtime` flag for paranoid mode (hash-based validation)<br>- Consider item count as secondary validation<br>- Default cache-max-age to prevent indefinite staleness |
| **Contingency** | Fall back to full scan if user reports inconsistencies |
| **Owner** | Software Engineer |

#### Risk 2: Cache Corruption

| Attribute | Details |
|-----------|---------|
| **Risk** | BadgerDB corruption loses all cache data |
| **Likelihood** | Low |
| **Impact** | Medium (need to rebuild cache, but no data loss) |
| **Detection** | BadgerDB open/read errors |
| **Mitigation** | - Graceful fallback to full scan on cache errors<br>- Log warnings but don't fail<br>- Regular cache validation checks<br>- Export/import cache backup feature (future) |
| **Contingency** | Delete corrupted cache and rebuild |
| **Owner** | Software Engineer |

#### Risk 3: Excessive Cache Size

| Attribute | Details |
|-----------|---------|
| **Risk** | Cache grows unbounded, consuming excessive disk space |
| **Likelihood** | Medium |
| **Impact** | Low (disk space) |
| **Detection** | User reports large cache directory |
| **Mitigation** | - Document expected cache size (~100MB per 1M files)<br>- Add cache cleanup command (`gdu --clear-cache`)<br>- Monitor cache size during testing<br>- Future: automatic eviction policies |
| **Contingency** | Manual cache cleanup instructions |
| **Owner** | Technical Writer |

#### Risk 4: Performance Regression

| Attribute | Details |
|-----------|---------|
| **Risk** | Incremental mode slower than regular scan (cache overhead) |
| **Likelihood** | Low |
| **Impact** | High (defeats purpose) |
| **Detection** | Benchmark tests in CI |
| **Mitigation** | - Comprehensive benchmarking before/after<br>- Performance tests in CI pipeline<br>- Keep regular mode as default<br>- Optimize BadgerDB operations (batch reads) |
| **Contingency** | Optimize or disable feature if regression detected |
| **Owner** | Software Engineer |

#### Risk 5: Schedule Delay

| Attribute | Details |
|-----------|---------|
| **Risk** | Implementation takes longer than 5 weeks |
| **Likelihood** | Medium |
| **Impact** | Medium (delayed release) |
| **Detection** | Weekly progress tracking |
| **Mitigation** | - Buffer time in estimates<br>- Parallel work where possible (Phase 2/3)<br>- De-scope optional features (TUI integration)<br>- Weekly status checks to catch slippage early |
| **Contingency** | Extend timeline or reduce scope (skip Phase 3 TUI integration) |
| **Owner** | Project Manager |

#### Risk 6: Insufficient Testing on Real NFS

| Attribute | Details |
|-----------|---------|
| **Risk** | Feature works in lab but fails on production NFS |
| **Likelihood** | Medium |
| **Impact** | High (production failures) |
| **Detection** | Production testing or early user reports |
| **Mitigation** | - Set up test NFS environment in Week 1<br>- Test with real NFS storage systems (DDN, NetApp)<br>- Beta release to early adopters<br>- Monitor GitHub issues closely after release |
| **Contingency** | Hot-fix release if critical issues found |
| **Owner** | QA Engineer |

### Risk Monitoring Approach

**Weekly Risk Review** (in weekly sync):
1. Review risk register
2. Update likelihood/impact based on progress
3. Escalate any high-priority risks
4. Add new risks as discovered

**Risk Indicators**:
- 🟢 Green: Risk unlikely or well-mitigated
- 🟡 Yellow: Risk possible, monitoring required
- 🔴 Red: Risk likely, immediate action needed

---

## 7. Testing & Quality Gates

### Test Coverage Requirements

| Category | Target Coverage | Enforcement |
|----------|----------------|-------------|
| Unit Tests | >85% | CI blocks merge if <85% |
| Integration Tests | All flag combinations | Manual checklist |
| Edge Case Tests | 8+ scenarios | Manual checklist |
| Benchmark Tests | 3+ scenarios | CI runs on each PR |

### Performance Benchmarking Requirements

#### Benchmark Scenarios

1. **Cold Cache** (first scan)
   - Expected: Similar performance to regular mode (±5%)
   - Measure: Total scan time, I/O operations

2. **Warm Cache, No Changes** (100% cache hit)
   - Expected: 10x-20x faster than full scan
   - Expected: 90%+ I/O reduction
   - Measure: Scan time, cache load time, I/O operations

3. **Warm Cache, 1% Changes** (realistic daily scenario)
   - Expected: 15x faster than full scan
   - Expected: 90%+ I/O reduction
   - Measure: Scan time, cache hit rate, I/O operations

#### Benchmark Datasets

- Small: 1,000 directories, 10,000 files (~100MB)
- Medium: 10,000 directories, 100,000 files (~1GB)
- Large: 100,000 directories, 1,000,000 files (~10GB)
- NFS: Real NFS mount with 1M+ files (production-like)

### Code Review Checkpoints

| Phase | Review Type | Checklist |
|-------|------------|-----------|
| Phase 1 | Architecture Review | - [ ] Design follows existing patterns<br>- [ ] Interfaces defined correctly<br>- [ ] BadgerDB integration sound |
| Phase 2 | Code Review | - [ ] Throttling implementation correct<br>- [ ] Thread-safety verified<br>- [ ] Tests adequate |
| Phase 3 | Code Review | - [ ] Statistics accurate<br>- [ ] Thread-safe counters<br>- [ ] Output formatting clear |
| Phase 4 | Security/Robustness Review | - [ ] All edge cases handled<br>- [ ] No panic scenarios<br>- [ ] Error messages helpful |
| Phase 5 | Final Review | - [ ] All tests passing<br>- [ ] Documentation complete<br>- [ ] No regressions<br>- [ ] Ready to merge |

### Quality Gates (Must Pass to Proceed)

**Gate 1: Phase 1 → Phase 2**
- [ ] Basic caching works (manual test passed)
- [ ] Unit tests pass with >80% coverage
- [ ] Code review approved
- [ ] No known blocker issues

**Gate 2: Phase 3 → Phase 4**
- [ ] Statistics accurate (validated by tests)
- [ ] CLI output formatted correctly
- [ ] Integration tests passing

**Gate 3: Phase 4 → Phase 5**
- [ ] All edge cases have tests
- [ ] No crashes in stress testing
- [ ] Code review approved

**Gate 4: Ready for Merge**
- [ ] All tests passing (unit, integration, benchmark)
- [ ] Code coverage >85%
- [ ] All linters passing
- [ ] Documentation complete
- [ ] Performance benchmarks meet targets
- [ ] Final code review approved
- [ ] No open blocker issues

---

## 8. Progress Tracking Template

### Weekly Status Update Template

**Week of**: [Date Range]
**Reporter**: [Name]

#### Summary
[2-3 sentence summary of week's progress]

#### Completed This Week
- [ ] Task 1
- [ ] Task 2
- [ ] Task 3

#### In Progress
- [ ] Task 4 (50% complete)
- [ ] Task 5 (just started)

#### Planned for Next Week
- [ ] Task 6
- [ ] Task 7

#### Metrics

| Metric | This Week | Target | Status |
|--------|-----------|--------|--------|
| Tasks Completed | X | Y | 🟢/🟡/🔴 |
| Code Lines Written | X | ~Y | 🟢/🟡/🔴 |
| Tests Written | X | Y | 🟢/🟡/🔴 |
| Test Coverage | X% | >85% | 🟢/🟡/🔴 |

#### Blockers/Issues
- [ ] Issue 1: [Description] - Assigned to [Name] - Due [Date]
- [ ] Issue 2: [Description] - Assigned to [Name] - Due [Date]

#### Risks
- 🟢 Risk 1: [Status]
- 🟡 Risk 2: [Status and mitigation]

#### Demo/Screenshots
[Include demo output, screenshots, or benchmark results if available]

#### Next Week's Focus
[1-2 sentence summary of priorities]

---

### Blockers/Issues Tracking Format

| ID | Issue | Impact | Owner | Status | Due Date | Resolution |
|----|-------|--------|-------|--------|----------|------------|
| B1 | NFS test environment not ready | High | DevOps | Open | Oct 10 | Waiting for IT |
| B2 | BadgerDB version compatibility | Medium | Engineer | In Progress | Oct 12 | Testing fix |
| I1 | Unit test flaky on macOS | Low | QA | Closed | Oct 9 | Fixed |

**Status Values**: Open, In Progress, Blocked, Closed

---

### Metrics to Track

#### Development Metrics (Weekly)

| Metric | Target | Tracking Method |
|--------|--------|-----------------|
| Tasks Completed | Per schedule | Manual checklist |
| Code Lines Written | ~700/week | `git diff --stat` |
| Tests Written | ~100/week | Test file line counts |
| Test Coverage | >85% | CI coverage report |
| Open Issues | <5 | GitHub issues |
| Blocker Issues | 0 | Issue tracker |

#### Quality Metrics (Per Phase)

| Metric | Target | Tracking Method |
|--------|--------|-----------------|
| Code Review Comments | <20 major | PR reviews |
| Bugs Found in Testing | <10 | Issue tracker |
| Performance Regressions | 0 | CI benchmarks |
| Linter Errors | 0 | CI linting |

#### End-to-End Metrics (Final)

| Metric | Target | Tracking Method |
|--------|--------|-----------------|
| I/O Reduction | >90% | Benchmark suite |
| Speedup (subsequent scans) | >15x | Benchmark suite |
| Memory Reduction | >50% | Memory profiler |
| Cache Hit Rate | >95% | Statistics output |
| Test Coverage | >85% | Coverage report |

---

## 9. Stakeholder Communication Plan

### Stakeholders

| Stakeholder | Role | Interest | Engagement Level |
|-------------|------|----------|------------------|
| **gdu Maintainers** | Project owners | Feature acceptance, code quality | High - weekly updates |
| **gdu Users** | End users | Feature functionality, documentation | Medium - release notes |
| **NFS Administrators** | Target users | Performance improvement | High - beta testing |
| **Project Sponsor** | Funding/approval | Timeline, business value | Medium - milestone updates |

### Communication Schedule

#### Weekly Updates (Internal Team)

**Frequency**: Every Friday, 2pm (suggested)
**Format**: 30-minute sync call + written status update
**Attendees**: Dev team, QA, Project Manager
**Agenda**:
1. Last week's progress (5 min)
2. This week's plan (5 min)
3. Blockers/issues (10 min)
4. Risk review (5 min)
5. Q&A (5 min)

**Deliverable**: Weekly status update (using template above)

#### Milestone Updates (Stakeholders)

**Frequency**: At each milestone completion (5 total)
**Format**: Email + demo (if applicable)
**Recipients**: Maintainers, sponsor, key users
**Content**:
- Milestone achieved
- Key capabilities demonstrated
- Next milestone preview
- Timeline status
- Demo video/screenshots (if applicable)

#### Monthly Summary (Sponsor)

**Frequency**: End of each month (2 total)
**Format**: Executive summary email
**Content**:
- Overall progress (% complete)
- Key achievements
- Risks and mitigation
- Timeline status
- Next month's focus

### Demo/Review Schedule

| Demo | Week | Audience | Content | Format |
|------|------|----------|---------|--------|
| **Demo 1: Core Working** | Week 1 | Internal team | Basic caching functionality | Screen share |
| **Demo 2: Stats Visible** | Week 3 | Maintainers | Statistics output | Screen share + data |
| **Demo 3: Final** | Week 5 | All stakeholders | Complete feature | Recorded demo + doc |

### Feedback Collection Process

**During Development** (Weeks 1-4):
- Collect feedback from maintainers via PR comments
- Log feature requests as future enhancements
- Address critical feedback within phase

**Beta Testing** (Week 5):
- Recruit 3-5 NFS users for beta testing
- Provide pre-release build
- Collect feedback via GitHub issues or form
- Address critical issues before release

**Post-Release** (After launch):
- Monitor GitHub issues closely (first 2 weeks)
- Weekly check-ins with early adopters
- Collect usage statistics (opt-in telemetry if available)

---

## 10. Post-Implementation Plan

### Deployment Strategy

#### Pre-Release Checklist

- [ ] All tests passing (100% pass rate)
- [ ] Code coverage >85%
- [ ] All linters passing
- [ ] Documentation complete and reviewed
- [ ] CHANGELOG.md updated
- [ ] Beta testing completed (3-5 users)
- [ ] Performance benchmarks meet targets
- [ ] No open blocker issues

#### Release Phases

**Phase A: Pre-release (Week 5)**
- Create pre-release tag (e.g., `v5.x.0-rc1`)
- Announce in GitHub discussions
- Request beta testers from community
- Monitor GitHub issues closely

**Phase B: Official Release (Week 6)**
- Merge feature branch to main
- Create release tag (e.g., `v5.x.0`)
- Publish release notes
- Update documentation site
- Announce on social media/forums

**Phase C: Post-Release Monitoring (Weeks 6-8)**
- Monitor GitHub issues daily
- Respond to user questions within 24h
- Hot-fix critical issues within 48h
- Collect usage feedback

### Rollback Plan

#### Rollback Triggers

- Critical bug causing data loss
- Performance regression >20%
- Widespread crashes/panics
- Cache corruption affecting multiple users

#### Rollback Procedure

1. **Immediate**:
   - Update README with "Known Issues" warning
   - Document workaround (use without `--incremental` flag)

2. **Short-term** (if needed):
   - Create hot-fix branch
   - Revert problematic code
   - Release patch version (e.g., `v5.x.1`)

3. **Long-term**:
   - Fix root cause in feature branch
   - Re-test thoroughly
   - Re-release when stable

**Note**: Since `--incremental` is opt-in, users can continue using gdu without the feature by simply not using the flag. This reduces rollback urgency.

### Post-Launch Monitoring

#### Metrics to Monitor (First 2 Weeks)

| Metric | Target | Alert Threshold | Check Frequency |
|--------|--------|-----------------|-----------------|
| GitHub Issues (bugs) | <5 | >10 | Daily |
| Crash Reports | 0 | >1 | Daily |
| Cache Corruption Reports | 0 | >1 | Immediate |
| Performance Complaints | 0 | >2 | Daily |
| Documentation Questions | <10 | >20 | Weekly |

#### Monitoring Channels

- GitHub Issues (labeled "incremental-caching")
- GitHub Discussions
- Community forums (if any)
- User feedback forms (if available)

### Success Evaluation (30 Days Post-Release)

**Quantitative Metrics**:
- [ ] >50 users report successful usage
- [ ] <5 bug reports total
- [ ] 0 critical issues
- [ ] Performance targets met (validated by users)
- [ ] >10 positive feedback comments

**Qualitative Metrics**:
- [ ] Users report significant time savings
- [ ] NFS administrators report reduced load
- [ ] Feature mentioned positively in reviews/articles
- [ ] Community adopts feature (config examples shared)

### Future Enhancement Roadmap

Based on implementation plan Section 10, potential Phase 6+ features:

#### Priority 1 (Next Release)
- [ ] Cache warming mode (`--incremental-warm`)
- [ ] Smart cache eviction (LRU, size limits)
- [ ] Cache cleanup command (`gdu --clear-cache`)

#### Priority 2 (Future)
- [ ] Distributed cache sharing (team collaboration)
- [ ] Predictive prefetching (access patterns)
- [ ] Compression for large caches

#### Priority 3 (Exploratory)
- [ ] Change notifications (inotify/FSEvents)
- [ ] Cloud storage support (S3, GCS)
- [ ] Machine learning for change prediction

**Decision Point**: Gather user feedback for 2-3 months, then prioritize enhancements based on:
- Most requested features
- Biggest pain points
- Technical feasibility
- Alignment with gdu's vision

---

## 11. Daily Task Tracking

### Week 1: Phase 1 Tasks

#### Monday, October 7
- [ ] Project kickoff meeting
- [ ] Set up development environment
- [ ] Create feature branch: `feature/incremental-caching`
- [ ] Task 1.1: Create `IncrementalAnalyzer` struct
- [ ] Task 1.2: Start implementing `processDir()` (partial)

#### Tuesday, October 8
- [ ] Task 1.2: Complete `processDir()` implementation
- [ ] Task 1.3: Create `IncrementalDirMetadata` schema
- [ ] Task 1.4: Start BadgerDB storage operations

#### Wednesday, October 9
- [ ] Task 1.4: Complete BadgerDB storage operations
- [ ] Task 1.5: Start cache loading and validation

#### Thursday, October 10
- [ ] Task 1.5: Complete cache loading and validation
- [ ] Task 1.6: Implement `rebuildFromCache()`
- [ ] Task 1.7: Add CLI flags

#### Friday, October 11
- [ ] Task 1.8: Write unit tests
- [ ] Task 1.9: Manual testing
- [ ] Week 1 demo and status update
- [ ] Quality Gate 1 review

---

### Week 2: Phase 2 Tasks

#### Monday, October 14
- [ ] Task 2.1: Design throttling architecture
- [ ] Task 2.2: Start implementing `IOThrottle` struct

#### Tuesday, October 15
- [ ] Task 2.2: Complete `IOThrottle` implementation
- [ ] Task 2.3: Integrate throttling into `processDir()`

#### Wednesday, October 16
- [ ] Task 2.4: Add `--max-iops` flag support
- [ ] Task 2.5: Add `--io-delay` flag support

#### Thursday, October 17
- [ ] Task 2.6: Write unit tests for throttling
- [ ] Task 2.7: Start benchmarking

#### Friday, October 18
- [ ] Task 2.7: Complete benchmarking
- [ ] Task 2.8: Test with parallel/sequential modes
- [ ] Week 2 status update

---

### Week 3: Phase 3 Tasks

#### Monday, October 21
- [ ] Task 3.1: Design `CacheStats` struct
- [ ] Task 3.2: Start implementing statistics collection

#### Tuesday, October 22
- [ ] Task 3.2: Complete statistics collection
- [ ] Task 3.3: Add `--show-cache-stats` flag

#### Wednesday, October 23
- [ ] Task 3.4: Implement CLI statistics output
- [ ] Task 3.5: Start TUI integration (optional)

#### Thursday, October 24
- [ ] Task 3.5: Complete TUI integration (optional)
- [ ] Task 3.6: Add cache metadata to JSON export

#### Friday, October 25
- [ ] Task 3.7: Write unit tests for statistics
- [ ] Demo 2: Statistics visible
- [ ] Week 3 status update
- [ ] Quality Gate 2 review

---

### Week 4: Phase 4 Tasks

#### Monday, October 28
- [ ] Task 4.1: Implement clock skew detection
- [ ] Task 4.2: Handle deleted directories/files
- [ ] Task 4.3: Handle moved/renamed directories

#### Tuesday, October 29
- [ ] Task 4.4: Handle permission changes
- [ ] Task 4.5: Start cache corruption detection

#### Wednesday, October 30
- [ ] Task 4.5: Complete cache corruption handling
- [ ] Task 4.6: Handle concurrent modifications

#### Thursday, October 31
- [ ] Task 4.7: Optimize large directory handling
- [ ] Task 4.8: Handle hardlink changes

#### Friday, November 1
- [ ] Task 4.9: Write edge case tests
- [ ] Task 4.10: Stress testing
- [ ] Week 4 status update
- [ ] Quality Gate 3 review

---

### Week 5: Phase 5 Tasks

#### Monday, November 4
- [ ] Task 5.1: Test with all existing flags
- [ ] Task 5.2: Test JSON export/import
- [ ] Task 5.3: Test interactive TUI mode
- [ ] Task 5.4: Test non-interactive mode

#### Tuesday, November 5
- [ ] Task 5.5: Start writing user guide
- [ ] Task 5.9: Start comprehensive benchmarking

#### Wednesday, November 6
- [ ] Task 5.5: Complete user guide
- [ ] Task 5.6: Update README.md
- [ ] Task 5.7: Update configuration.md
- [ ] Task 5.9: Complete benchmarking

#### Thursday, November 7
- [ ] Task 5.8: Create example configurations
- [ ] Task 5.10: Write integration tests
- [ ] Task 5.11: Final code review and cleanup

#### Friday, November 8
- [ ] Task 5.12: Update CHANGELOG.md
- [ ] Demo 3: Final feature demonstration
- [ ] Quality Gate 4 review
- [ ] Create PR for merge
- [ ] Week 5 final status update

---

## 12. Quick Reference Checklists

### Pre-Phase Checklist (Before Starting Any Phase)

- [ ] Previous phase complete (or N/A for Phase 1)
- [ ] Quality gate passed (if applicable)
- [ ] Development environment ready
- [ ] Test data prepared
- [ ] Team members available
- [ ] Blockers cleared

### Per-Phase Completion Checklist

- [ ] All tasks completed
- [ ] Unit tests written and passing
- [ ] Code coverage meets target (>85% for final, >80% per phase)
- [ ] Manual testing completed
- [ ] Code reviewed and approved
- [ ] Documentation updated (if applicable)
- [ ] Phase demo completed (if scheduled)
- [ ] Quality gate passed
- [ ] Status update sent

### Pre-Merge Checklist (End of Phase 5)

**Code Quality**:
- [ ] All unit tests passing (100%)
- [ ] All integration tests passing (100%)
- [ ] All benchmark tests passing
- [ ] Code coverage >85%
- [ ] All linters passing (golangci-lint)
- [ ] No compiler warnings

**Functionality**:
- [ ] All acceptance criteria met (Phases 1-5)
- [ ] All edge cases handled
- [ ] Performance targets met (90%+ I/O reduction, 15x speedup)
- [ ] Compatibility matrix verified

**Documentation**:
- [ ] `docs/incremental-caching.md` complete
- [ ] `README.md` updated
- [ ] `configuration.md` updated
- [ ] `CHANGELOG.md` updated
- [ ] Example configurations created
- [ ] Code comments sufficient

**Testing**:
- [ ] Unit tests cover all new code
- [ ] Integration tests cover all flag combinations
- [ ] Edge case tests cover 8+ scenarios
- [ ] Benchmark tests run successfully
- [ ] Manual NFS testing completed

**Review**:
- [ ] Code review completed and approved
- [ ] Security/robustness review completed
- [ ] Documentation review completed
- [ ] No open blocker issues
- [ ] All feedback addressed

**Release**:
- [ ] PR created with detailed description
- [ ] CI/CD pipeline passing
- [ ] Beta testers identified (3-5 users)
- [ ] Release notes drafted

---

## 13. Contact Information

| Role | Name | Contact | Availability |
|------|------|---------|--------------|
| **Project Lead** | TBD | TBD | TBD |
| **Software Engineer** | TBD | TBD | TBD |
| **QA Engineer** | TBD | TBD | TBD |
| **Technical Writer** | TBD | TBD | TBD |
| **Code Reviewer** | TBD | TBD | TBD |
| **Project Manager** | TBD | TBD | TBD |
| **Maintainer (gdu)** | [dundee](https://github.com/dundee) | GitHub: @dundee | Check GitHub |

---

## 14. Tools & Resources

### Development Tools

| Tool | Purpose | Installation |
|------|---------|--------------|
| **Go 1.23+** | Implementation language | [golang.org](https://golang.org/dl/) |
| **BadgerDB** | Cache storage | `go get github.com/dgraph-io/badger/v4` |
| **golangci-lint** | Linting | `make install-dev-dependencies` |
| **gotestsum** | Test runner | `make install-dev-dependencies` |
| **gox** | Cross-compilation | `go install github.com/mitchellh/gox@latest` |

### Testing Tools

| Tool | Purpose | Usage |
|------|---------|-------|
| **go test** | Unit testing | `go test ./...` |
| **go test -bench** | Benchmarking | `make gobench` |
| **go test -race** | Race detection | `go test -race ./...` |
| **pprof** | Profiling | `make heap-profile` |

### Documentation Tools

| Tool | Purpose | Usage |
|------|---------|-------|
| **Markdown** | Documentation format | Standard markdown syntax |
| **GitHub** | Code hosting + issues | [github.com/dundee/gdu](https://github.com/dundee/gdu) |

### Test Environments

| Environment | Purpose | Setup |
|------------|---------|-------|
| **Local Filesystem** | Basic testing | Use `/tmp` or test directories |
| **NFS Mount** | Production-like testing | Mount NFS share in Week 1 |
| **Large Dataset** | Performance testing | Generate or acquire 1M+ file dataset |

---

## 15. Appendix

### A. Glossary

| Term | Definition |
|------|------------|
| **Incremental Caching** | Only re-scanning directories that have changed based on mtime comparison |
| **mtime** | Modification time - filesystem timestamp indicating when file/directory was last modified |
| **BadgerDB** | Embedded key-value database used for persistent cache storage |
| **Cache Hit** | When cached data is used instead of scanning filesystem |
| **Cache Miss** | When directory must be scanned because no valid cache entry exists |
| **Cache Expiry** | Cache entry is too old (exceeds --cache-max-age) and must be refreshed |
| **I/O Throttling** | Rate limiting filesystem operations to reduce storage load |
| **IOPS** | I/O Operations Per Second - measure of filesystem operation rate |
| **NFS** | Network File System - network protocol for distributed filesystems |
| **DDN** | DataDirect Networks - high-performance storage systems |

### B. Command Reference

```bash
# Basic incremental caching
gdu --incremental /path

# With cache expiry (24 hours)
gdu --incremental --cache-max-age 24h /path

# With I/O throttling (1000 operations/second)
gdu --incremental --max-iops 1000 /path

# With fixed delay between scans
gdu --incremental --io-delay 10ms /path

# Force full scan and update cache
gdu --incremental --force-full-scan /path

# Show cache statistics
gdu --incremental --show-cache-stats /path

# Custom cache location
gdu --incremental --incremental-path /custom/path /scan/path

# Combined with other flags
gdu --incremental --no-cross --ignore-dirs "node_modules,vendor" /path
```

### C. File Size Estimates

| File | Lines | Estimated Size |
|------|-------|----------------|
| `pkg/analyze/incremental.go` | ~500 | ~15 KB |
| `pkg/analyze/incremental_storage.go` | ~300 | ~9 KB |
| `pkg/analyze/incremental_stats.go` | ~150 | ~4 KB |
| `pkg/analyze/throttle.go` | ~100 | ~3 KB |
| `pkg/analyze/incremental_test.go` | ~600 | ~18 KB |
| `pkg/analyze/incremental_bench_test.go` | ~200 | ~6 KB |
| `pkg/analyze/incremental_storage_test.go` | ~300 | ~9 KB |
| `cmd/gdu/app/app_incremental_test.go` | ~400 | ~12 KB |
| `docs/incremental-caching.md` | ~1000 | ~30 KB |
| **Total** | **~3,550** | **~106 KB** |

### D. External Dependencies

| Dependency | Version | Purpose |
|------------|---------|---------|
| `github.com/dgraph-io/badger/v4` | v4.x | Cache storage |
| `golang.org/x/time/rate` | latest | Rate limiting |
| Standard library | - | Core implementation |

### E. Related Documentation

- [Implementation Plan](/Users/zoran.vukmirica.889/github/gdu/docs/incremental-caching-implementation-plan.md)
- [gdu README](../README.md)
- [Configuration Guide](../configuration.md)
- [CLAUDE.md (Project Instructions)](../CLAUDE.md)

---

## Document Metadata

| Attribute | Value |
|-----------|-------|
| **Version** | 1.0 |
| **Created** | 2025-10-02 |
| **Last Updated** | 2025-10-02 |
| **Status** | Active |
| **Owner** | Project Manager (TBD) |
| **Next Review** | Weekly during implementation |

---

**End of Project Tracker**
