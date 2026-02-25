package providers

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/justchokingaround/greg/internal/config"
)

// Registry manages registered providers and their health statuses
type Registry struct {
	mu        sync.RWMutex
	providers map[string]Provider
	byType    map[MediaType][]Provider
	statuses  map[string]*ProviderStatus
}

var (
	// globalRegistry is the default provider registry
	globalRegistry = NewRegistry()
)

// NewRegistry creates a new provider registry
func NewRegistry() *Registry {
	return &Registry{
		providers: make(map[string]Provider),
		byType:    make(map[MediaType][]Provider),
		statuses:  make(map[string]*ProviderStatus),
	}
}

// Register adds a provider to the registry
func (r *Registry) Register(provider Provider) error {
	if provider == nil {
		return fmt.Errorf("cannot register nil provider")
	}

	name := provider.Name()
	if name == "" {
		return fmt.Errorf("provider must have a name")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if already registered
	if _, exists := r.providers[name]; exists {
		return fmt.Errorf("provider %s is already registered", name)
	}

	// Register by name
	r.providers[name] = provider

	// Initialize status
	r.statuses[name] = &ProviderStatus{
		ProviderName: name,
		Status:       "Pending",
	}

	// Register by type
	providerType := provider.Type()
	r.byType[providerType] = append(r.byType[providerType], provider)

	// If provider supports multiple types (e.g., "All"), add to all types
	switch providerType {
	case MediaTypeAll:
		r.byType[MediaTypeAnime] = append(r.byType[MediaTypeAnime], provider)
		r.byType[MediaTypeMovie] = append(r.byType[MediaTypeMovie], provider)
		r.byType[MediaTypeTV] = append(r.byType[MediaTypeTV], provider)
		r.byType[MediaTypeMovieTV] = append(r.byType[MediaTypeMovieTV], provider)
	case MediaTypeMovieTV:
		r.byType[MediaTypeMovie] = append(r.byType[MediaTypeMovie], provider)
		r.byType[MediaTypeTV] = append(r.byType[MediaTypeTV], provider)
	case MediaTypeAnimeMovieTV:
		r.byType[MediaTypeAnime] = append(r.byType[MediaTypeAnime], provider)
		r.byType[MediaTypeMovie] = append(r.byType[MediaTypeMovie], provider)
		r.byType[MediaTypeTV] = append(r.byType[MediaTypeTV], provider)
		r.byType[MediaTypeMovieTV] = append(r.byType[MediaTypeMovieTV], provider)
	}

	return nil
}

// Unregister removes a provider from the registry
func (r *Registry) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	provider, exists := r.providers[name]
	if !exists {
		return fmt.Errorf("provider %s is not registered", name)
	}

	// Remove from name map, statuses, and type map
	delete(r.providers, name)
	delete(r.statuses, name)
	providerType := provider.Type()
	r.byType[providerType] = removeProvider(r.byType[providerType], provider)

	// Handle multiple types
	switch providerType {
	case MediaTypeAll:
		r.byType[MediaTypeAnime] = removeProvider(r.byType[MediaTypeAnime], provider)
		r.byType[MediaTypeMovie] = removeProvider(r.byType[MediaTypeMovie], provider)
		r.byType[MediaTypeTV] = removeProvider(r.byType[MediaTypeTV], provider)
		r.byType[MediaTypeMovieTV] = removeProvider(r.byType[MediaTypeMovieTV], provider)
	case MediaTypeMovieTV:
		r.byType[MediaTypeMovie] = removeProvider(r.byType[MediaTypeMovie], provider)
		r.byType[MediaTypeTV] = removeProvider(r.byType[MediaTypeTV], provider)
	case MediaTypeAnimeMovieTV:
		r.byType[MediaTypeAnime] = removeProvider(r.byType[MediaTypeAnime], provider)
		r.byType[MediaTypeMovie] = removeProvider(r.byType[MediaTypeMovie], provider)
		r.byType[MediaTypeTV] = removeProvider(r.byType[MediaTypeTV], provider)
		r.byType[MediaTypeMovieTV] = removeProvider(r.byType[MediaTypeMovieTV], provider)
	}

	return nil
}

// Get returns a provider by name
func (r *Registry) Get(name string) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	provider, exists := r.providers[name]
	if !exists {
		return nil, fmt.Errorf("provider %s not found", name)
	}

	return provider, nil
}

// GetByType returns all providers that support the given media type
func (r *Registry) GetByType(mediaType MediaType) []Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()

	providers := r.byType[mediaType]
	// Return a copy to prevent external modification
	result := make([]Provider, len(providers))
	copy(result, providers)
	return result
}

// GetAll returns all registered providers
func (r *Registry) GetAll() []Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Provider, 0, len(r.providers))
	for _, provider := range r.providers {
		result = append(result, provider)
	}
	return result
}

// List returns the names of all registered providers
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.providers))
	for name := range r.providers {
		names = append(names, name)
	}
	return names
}

// Count returns the number of registered providers
func (r *Registry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.providers)
}

// Clear removes all providers from the registry
func (r *Registry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.providers = make(map[string]Provider)
	r.byType = make(map[MediaType][]Provider)
	r.statuses = make(map[string]*ProviderStatus)
}

// formatCurlCommand generates a curl command for debugging
func formatCurlCommand(url string, headers map[string]string) string {
	var b strings.Builder
	b.WriteString("curl -v ")
	for k, v := range headers {
		b.WriteString(fmt.Sprintf("-H '%s: %s' ", k, v))
	}
	b.WriteString(fmt.Sprintf("'%s'", url))
	return b.String()
}

// CheckAllProviders runs a health check on all registered providers concurrently.
func (r *Registry) CheckAllProviders(ctx context.Context) {
	providers := r.GetAll()
	var wg sync.WaitGroup

	for _, p := range providers {
		wg.Add(1)
		go func(provider Provider) {
			defer wg.Done()

			// Initial status
			r.mu.Lock()
			r.statuses[provider.Name()].Status = "Checking..."
			r.statuses[provider.Name()].LastCheck = time.Now()
			r.mu.Unlock()

			// Map provider names to typical URLs for health check
			urlMap := map[string]string{
				"hianime":  "https://hianime.to",
				"allanime": "https://allanime.to",
				"sflix":    "https://sflixto.uk",
				"flixhq":   "https://flixhq.to",
				"hdrezka":  "https://hdrezka.me",
				"comix":    "https://comick.io",
			}

			healthURL := urlMap[provider.Name()]
			if healthURL == "" {
				healthURL = "https://unknown"
			}

			// Run health check with a timeout
			checkCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()

			startTime := time.Now()
			err := provider.HealthCheck(checkCtx)
			duration := time.Since(startTime)

			// Build health check result
			headers := map[string]string{
				"User-Agent": "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36",
			}
			curlCmd := formatCurlCommand(healthURL, headers)

			result := &HealthCheckResult{
				URL:         healthURL,
				CurlCommand: curlCmd,
				Duration:    duration,
				CheckedAt:   time.Now(),
			}

			// Update status
			r.mu.Lock()
			defer r.mu.Unlock()
			if err != nil {
				r.statuses[provider.Name()].Healthy = false
				r.statuses[provider.Name()].Status = fmt.Sprintf("Offline: %v", err)
				result.Error = err.Error()
				result.StatusCode = 0
			} else {
				r.statuses[provider.Name()].Healthy = true
				r.statuses[provider.Name()].Status = "Online"
				result.StatusCode = 200
			}
			r.statuses[provider.Name()].LastCheck = time.Now()
			r.statuses[provider.Name()].LastResult = result
		}(p)
	}

	wg.Wait()
}

// GetProviderStatuses returns the health status of all registered providers.
func (r *Registry) GetProviderStatuses() []*ProviderStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()

	statuses := make([]*ProviderStatus, 0, len(r.statuses))
	for _, status := range r.statuses {
		statuses = append(statuses, status)
	}
	// Sort for consistent ordering
	sort.Slice(statuses, func(i, j int) bool {
		return statuses[i].ProviderName < statuses[j].ProviderName
	})
	return statuses
}

// Reconfigure updates providers based on configuration
func (r *Registry) Reconfigure(cfg *config.Config, logger *slog.Logger) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Helper to swap implementation
	swap := func(name string, mode string, url string, pType MediaType) {
		if mode == "remote" {
			// Instantiate Remote adapter
			// We use a temporary config for the remote provider
			remoteCfg := &config.Config{}
			remoteCfg.API.BaseURL = url
			remoteCfg.API.Timeout = 30 * time.Second // Default timeout

			// Create remote provider
			// Note: NewRemoteProvider is defined in remote_provider.go in the same package
			remoteProvider := NewRemoteProvider(name, pType, remoteCfg, logger)

			// Update provider map
			r.providers[name] = remoteProvider

			// Update type maps
			// First remove existing
			if existing, ok := r.providers[name]; ok {
				existingType := existing.Type()
				r.byType[existingType] = removeProvider(r.byType[existingType], existing)

				switch existingType {
				case MediaTypeAll, MediaTypeAnimeMovieTV:
					r.byType[MediaTypeAnime] = removeProvider(r.byType[MediaTypeAnime], existing)
					r.byType[MediaTypeMovie] = removeProvider(r.byType[MediaTypeMovie], existing)
					r.byType[MediaTypeTV] = removeProvider(r.byType[MediaTypeTV], existing)
					r.byType[MediaTypeMovieTV] = removeProvider(r.byType[MediaTypeMovieTV], existing)
				case MediaTypeMovieTV:
					r.byType[MediaTypeMovie] = removeProvider(r.byType[MediaTypeMovie], existing)
					r.byType[MediaTypeTV] = removeProvider(r.byType[MediaTypeTV], existing)
				}
			}

			// Then add new
			r.byType[pType] = append(r.byType[pType], remoteProvider)

			switch pType {
			case MediaTypeAll, MediaTypeAnimeMovieTV:
				r.byType[MediaTypeAnime] = append(r.byType[MediaTypeAnime], remoteProvider)
				r.byType[MediaTypeMovie] = append(r.byType[MediaTypeMovie], remoteProvider)
				r.byType[MediaTypeTV] = append(r.byType[MediaTypeTV], remoteProvider)
				r.byType[MediaTypeMovieTV] = append(r.byType[MediaTypeMovieTV], remoteProvider)
			case MediaTypeMovieTV:
				r.byType[MediaTypeMovie] = append(r.byType[MediaTypeMovie], remoteProvider)
				r.byType[MediaTypeTV] = append(r.byType[MediaTypeTV], remoteProvider)
			}

			// Update status
			r.statuses[name] = &ProviderStatus{
				ProviderName: name,
				Status:       "Remote Configured",
				Healthy:      true,
			}
		}
		// If local, we assume the init() function already registered the local version,
		// or we re-instantiate the local version if needed.
		// Since we don't have access to local provider constructors here without circular deps,
		// we assume local is the default and only swap if remote is requested.
		// If switching back to local is required, a restart would be needed currently.
	}

	// Apply for all known providers
	swap("hdrezka", cfg.Providers.HDRezka.Mode, cfg.Providers.HDRezka.RemoteURL, MediaTypeAnimeMovieTV)
	swap("hianime", cfg.Providers.HiAnime.Mode, cfg.Providers.HiAnime.RemoteURL, MediaTypeAnime)
	swap("allanime", cfg.Providers.AllAnime.Mode, cfg.Providers.AllAnime.RemoteURL, MediaTypeAnime)
	swap("sflix", cfg.Providers.SFlix.Mode, cfg.Providers.SFlix.RemoteURL, MediaTypeMovieTV)
	swap("flixhq", cfg.Providers.FlixHQ.Mode, cfg.Providers.FlixHQ.RemoteURL, MediaTypeMovieTV)
}

// Helper function to remove a provider from a slice
func removeProvider(providers []Provider, target Provider) []Provider {
	result := make([]Provider, 0, len(providers))
	for _, p := range providers {
		if p.Name() != target.Name() {
			result = append(result, p)
		}
	}
	return result
}

// Global registry functions

// Register adds a provider to the global registry
func Register(provider Provider) error {
	return globalRegistry.Register(provider)
}

// Unregister removes a provider from the global registry
func Unregister(name string) error {
	return globalRegistry.Unregister(name)
}

// Get returns a provider by name from the global registry
func Get(name string) (Provider, error) {
	return globalRegistry.Get(name)
}

// GetByType returns all providers that support the given media type from the global registry
func GetByType(mediaType MediaType) []Provider {
	return globalRegistry.GetByType(mediaType)
}

// Configurable is an interface for providers that can be configured at runtime
type Configurable interface {
	SetConfig(cfg *config.Config, logger *slog.Logger)
}

// ConfigureAll configures all registered providers that implement the Configurable interface
func ConfigureAll(cfg *config.Config, logger *slog.Logger) {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()

	for _, provider := range globalRegistry.providers {
		if configurable, ok := provider.(Configurable); ok {
			configurable.SetConfig(cfg, logger)
		}
	}
}

// GetAll returns all registered providers from the global registry
func GetAll() []Provider {
	return globalRegistry.GetAll()
}

// List returns the names of all registered providers from the global registry
func List() []string {
	return globalRegistry.List()
}

// Count returns the number of registered providers in the global registry
func Count() int {
	return globalRegistry.Count()
}

// Clear removes all providers from the global registry
func Clear() {
	globalRegistry.Clear()
}

// GetRegistry returns the global registry instance
func GetRegistry() *Registry {
	return globalRegistry
}

// CheckAllProviders runs health checks on all providers in the global registry.
func CheckAllProviders(ctx context.Context) {
	globalRegistry.CheckAllProviders(ctx)
}

// GetProviderStatuses returns the health statuses from the global registry.
func GetProviderStatuses() []*ProviderStatus {
	return globalRegistry.GetProviderStatuses()
}
