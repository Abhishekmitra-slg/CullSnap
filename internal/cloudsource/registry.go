package cloudsource

// Registry holds all registered cloud source providers.
// Maintains insertion order for deterministic UI rendering.
type Registry struct {
	providers map[string]CloudSource
	order     []string
}

func NewRegistry() *Registry {
	return &Registry{providers: make(map[string]CloudSource)}
}

func (r *Registry) Register(source CloudSource) {
	id := source.ID()
	if _, exists := r.providers[id]; !exists {
		r.order = append(r.order, id)
	}
	r.providers[id] = source
}

func (r *Registry) Get(providerID string) (CloudSource, bool) {
	s, ok := r.providers[providerID]
	return s, ok
}

func (r *Registry) All() []CloudSourceStatus {
	statuses := make([]CloudSourceStatus, 0, len(r.order))
	for _, id := range r.order {
		s := r.providers[id]
		statuses = append(statuses, CloudSourceStatus{
			ProviderID:  s.ID(),
			DisplayName: s.DisplayName(),
			IsAvailable: s.IsAvailable(),
			IsConnected: s.IsAuthenticated(),
		})
	}
	return statuses
}
