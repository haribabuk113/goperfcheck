package checker

// AllCheckers returns every performance rule checker based on goperf.dev.
func AllCheckers() []Checker {
	return []Checker{
		&MemPreallocChecker{},
		&ObjectPoolChecker{},
		&StructAlignChecker{},
		&InterfaceBoxingChecker{},
		&ZeroCopyChecker{},
		&GoroutinePoolChecker{},
		&ContextMisuseChecker{},
		&BufferedIOChecker{},
		&AtomicMutexChecker{},
		&LazyInitChecker{},
		&StackAllocChecker{},
		&BatchingChecker{},
		&TimeNowLoopChecker{},
		&WaitGroupMisuseChecker{},
	}
}
