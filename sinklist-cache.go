package main

import (
	"strings"
	"sync"
)

type tAction byte

const (
	ActionBlack = tAction(Action_BLACK)
	ActionWhite = tAction(Action_WHITE)
	ActionLog   = tAction(Action_LOG)
	ActionCheck = tAction(Action_CHECK)
)

// ListCache contains 3 SinklistCache objects for storing Core data
type ListCache struct {
	Customlist		SinklistCache
	Ioclist			SinklistCache
	// Pertinent for cloud resolver, holds really all IoCs and all Custom lists regardless of any setting, i.e. for everybody
	AllIoCwithCustomLists	SinklistCache
}

// SinklistCache represents one of caches of Core data - whitelist, customlist, ioclist
type SinklistCache interface {
	Get(key string) (action tAction, err error)
	Set(key string, action tAction) error
	Exists(key string) bool
	Remove(key string)
	Length() int
	Replace(cache map[string]tAction)
	FindFirstKey(keys []string) (action tAction, err error)
}

// SinklistMemoryCache is SinklistCache with memory backend
type SinklistMemoryCache struct {
	Backend map[string]tAction
	mu      sync.RWMutex
}

// NewListCache creates object for storing whitelist/customlist/ioclist cache
func NewListCache() *ListCache {
	return &ListCache{
		Customlist:            NewSinklistMemoryCache(),
		Ioclist:               NewSinklistMemoryCache(),
		AllIoCwithCustomLists: NewSinklistMemoryCache(),
	}
}

// NewSinklistMemoryCache creates new SinklistCache with memory backend
func NewSinklistMemoryCache() *SinklistMemoryCache {
	return &SinklistMemoryCache{
		Backend: make(map[string]tAction),
	}
}

func (c *SinklistMemoryCache) Get(key string) (tAction, error) {
	logger.Debug("SinklistCache Get: called for key: %s", key)
	c.mu.RLock()
	data, ok := c.Backend[key]
	c.mu.RUnlock()
	if !ok {
		logger.Debug("SinklistCache Get: key: %s was not found.", key)
		return ActionBlack, KeyNotFound{key}
	}

	logger.Debug("SinklistCache Get: key: %s found value: %t", key, data)
	return data, nil
}

func (c *SinklistMemoryCache) Set(key string, action tAction) error {
	logger.Debug("SinklistCache Set: key: %s, value: %t", key, action)
	c.mu.Lock()
	c.Backend[key] = action
	c.mu.Unlock()
	return nil
}

// Replace whole content of cache in atomic operation
func (c *SinklistMemoryCache) Replace(cache map[string]tAction) {
	logger.Debug("SinklistCache Replace")
	c.mu.Lock()
	c.Backend = cache
	c.mu.Unlock()
}

func (c *SinklistMemoryCache) Remove(key string) {
	logger.Debug("SinklistCache Remove: key: %s was removed.", key)
	c.mu.Lock()
	delete(c.Backend, key)
	c.mu.Unlock()
}

func (c *SinklistMemoryCache) Exists(key string) bool {
	c.mu.RLock()
	_, ok := c.Backend[key]
	c.mu.RUnlock()
	logger.Debug("SinklistCache Exists: key: %s exists: %t", key, ok)
	return ok
}

func (c *SinklistMemoryCache) Length() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.Backend)
}

// FindFirstKey finds first existing key from array of strings, error if none exist
func (c *SinklistMemoryCache) FindFirstKey(keys []string) (action tAction, err error) {
	logger.Debug("SinklistCache Get: called for keys: %s", strings.Join(keys, ","))
	c.mu.RLock()
	for _, key := range keys {
		data, ok := c.Backend[key]
		if ok {
			logger.Debug("SinklistCache Get: key: %s found value: %t", key, data)
			c.mu.RUnlock()
			return data, nil
		}
	}
	c.mu.RUnlock()
	logger.Debug("SinklistCache Get: keys: %s were not found.", keys)
	return ActionBlack, KeyNotFound{strings.Join(keys, ",")}
}
