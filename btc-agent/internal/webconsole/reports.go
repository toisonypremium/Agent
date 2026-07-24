package webconsole

// ReportCatalog is default-deny. An empty catalog is intentional until a report
// receives a separately reviewed fixed-ID allowlist entry and renderer policy.
type ReportCatalog struct {
	Reports []ReportDescriptor `json:"reports"`
}
type ReportDescriptor struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	GeneratedAt string `json:"generated_at"`
}

func (s *Service) Reports() ReportCatalog { return ReportCatalog{Reports: []ReportDescriptor{}} }
