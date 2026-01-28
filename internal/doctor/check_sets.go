package doctor

// SessionGCChecks returns the minimal set of checks needed for session garbage collection.
func SessionGCChecks() []Check {
	return []Check{
		NewOrphanSessionCheck(),
		NewZombieSessionCheck(),
		NewOrphanProcessCheck(),
		NewWispGCCheck(),
	}
}
