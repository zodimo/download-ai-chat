package providers

import (
	"fmt"
	"sync"
)

var localRegistry Registry

type Registry interface {
	Register(name string, platform Platform)
	Get(name string) (Platform, error)
	Has(name string) bool
	GetAll() []Platform
}

var _ Registry = (*registry)(nil)

type registry struct {
	platforms sync.Map
}

func (r *registry) Register(name string, platform Platform) {
	r.platforms.Store(name, platform)
}

func (r *registry) Get(name string) (Platform, error) {
	raw, exists := r.platforms.Load(name)
	if exists {
		if platform, ok := raw.(Platform); ok {
			return platform, nil
		} else {
			var zero Platform
			return nil, fmt.Errorf("Expected %T, got %T", zero, raw)
		}
	}
	return nil, fmt.Errorf("No provider registed for key: %s", name)
}

func (r *registry) Has(name string) bool {
	_, ok := r.platforms.Load(name)
	return ok
}

func (r *registry) GetAll() []Platform {
	platforms := make([]Platform, 0)
	r.platforms.Range(func(key, value any) bool {
		platforms = append(platforms, value.(Platform))
		return true
	})
	return platforms
}

func init() {
	localRegistry = &registry{}
}

func Register(name string, platform Platform) {
	localRegistry.Register(name, platform)
}

func GetPlatform(name string) (Platform, error) {
	return localRegistry.Get(name)
}

func GetAllPlatforms() []Platform {
	return localRegistry.GetAll()
}

func HasPlatform(name string) bool {
	return localRegistry.Has(name)
}
