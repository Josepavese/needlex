package analytics

import "context"

// Recorder accepts a completed observation. The adapter owns its transaction
// and releases database resources before AppendRun returns.
type Recorder interface {
	AppendRun(context.Context, RunRecord, []StageEvent) error
}

// Reports provides analytics views and exports without exposing a database
// connection. Calls honor the caller's context and own no resource after return.
type Reports interface {
	DBPath() string
	Stats(context.Context) (Stats, error)
	RecentRuns(context.Context, int) ([]RecentRun, error)
	ValueReport(context.Context) (ValueReport, error)
	Hosts(context.Context, int) ([]HostRollup, error)
	Providers(context.Context, int) ([]ProviderRollup, error)
	Failures(context.Context, int) ([]FailureRollup, error)
	Daily(context.Context, int) ([]DailyRollup, error)
	ExportJSON(context.Context, string) (ExportStats, error)
}

// Store is the composition contract for the local analytics capability.
// Factories return an operation-scoped handle, used synchronously by its caller.
type Store interface {
	Recorder
	Reports
}

var _ Store = SQLiteStore{}
