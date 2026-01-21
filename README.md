# vlru

A Go LRU cache library with distributed cache invalidation, based on [hashicorp/golang-lru](https://github.com/hashicorp/golang-lru).

## Features

- Drop-in replacement for `hashicorp/golang-lru` with the same API
- Distributed cache invalidation via pub/sub
- Support for both standard LRU and expirable (TTL-based) caches
- Pluggable broker interface for different message transports
- Built-in serializers for string and JSON keys
- Cascade prevention (events don't re-publish)

## Installation

```bash
go get github.com/vogo/vlru
```

## Quick Start

```go
package main

import (
    "github.com/vogo/vlru"
    "github.com/vogo/vlru/examples/inmemory"
    "github.com/vogo/vogo/vsync/vrun"
)

func main() {
    // Configure global broker (once at startup)
    runner := vrun.New()
    defer runner.Stop()

    broker := inmemory.New()
    vlru.StartEventBroker(runner, broker)

    // Create cache - same API as hashicorp/golang-lru
    // Cache name is auto-generated from call site (file:function:line)
    cache, _ := vlru.New[string, User](1000)
    defer cache.Close()

    // Use normally - invalidations auto-propagate
    cache.Add("user:1", user)
    cache.Remove("user:1")  // publishes event, other instances remove key
}
```

## How It Works

When one cache instance evicts or removes a key, it publishes an invalidation event via the configured Broker. Other instances receive the event and remove the key locally without re-publishing (preventing cascade).

### Event Flow

**Publish Path (local eviction → remote invalidation):**
```
Add() causes eviction → onEvict callback
  → serialize key
  → Broker.Publish(event)
  → other instances receive via their broker
  → broker routes to registry
  → registry calls cache.InvalidateKey()
```

**Receive Path (remote event → local removal):**
```
Broker receives event from network
  → routes to registry
  → registry checks InstanceID != self
  → finds cache by CacheName
  → cache.InvalidateKey() with suppressPublish=true
  → NO cascade event published
```

## API Reference

### Configuration

```go
// StartEventBroker sets the global broker and starts the event loop
runner := vrun.New()
vlru.StartEventBroker(runner, broker)
```

### Cache Naming

Cache names are auto-generated from the call site (file:function:line). This means:
- Same code location → same cache name → events sync correctly
- Different code locations → different cache names → events don't cross

For distributed invalidation to work between multiple cache instances, they must share the same cache name. Use `WithCacheName` to override the auto-generated name:

```go
// Two caches that should sync must have the same name
cache1, _ := vlru.New[string, User](1000, vlru.WithCacheName[string, User]("users"))
cache2, _ := vlru.New[string, User](1000, vlru.WithCacheName[string, User]("users"))
```

### Standard LRU Cache

```go
// Create a cache with capacity of 1000 items
// Cache name is auto-generated from call site
cache, err := vlru.New[string, User](1000)

// Create with eviction callback
cache, err := vlru.NewWithEvict[string, User](1000, func(key string, value User) {
    log.Printf("evicted: %s", key)
})

// With custom options
cache, err := vlru.New[string, User](1000,
    vlru.WithCacheName[string, User]("users"),      // override auto-name
    vlru.WithSerializer[string, User](vlru.StringKeySerializer{}),
)
```

### Expirable LRU Cache (with TTL)

```go
import "github.com/vogo/vlru/vexpirable"

// Create cache with 5-minute TTL
cache := vexpirable.NewLRU[string, User](1000, nil, 5*time.Minute)
defer cache.Close()

// With eviction callback
cache := vexpirable.NewLRU[string, User](1000, func(key string, value User) {
    log.Printf("expired or evicted: %s", key)
}, 5*time.Minute)

// With custom options
cache := vexpirable.NewLRU[string, User](1000, nil, 5*time.Minute,
    vexpirable.WithCacheName[string, User]("users"),
)
```

### Cache Methods

All standard `hashicorp/golang-lru` methods are supported:

```go
cache.Add(key, value)           // Add or update
cache.Get(key)                  // Get with LRU update
cache.Peek(key)                 // Get without LRU update
cache.Contains(key)             // Check existence
cache.Remove(key)               // Remove (publishes event)
cache.Len()                     // Current size
cache.Keys()                    // All keys
cache.Values()                  // All values
cache.Purge()                   // Clear all
cache.Resize(newSize)           // Change capacity
cache.Close()                   // Cleanup and unregister
```

## Implementing a Broker

The `Broker` interface handles event publishing and receiving:

```go
type Broker interface {
    // Publish sends an invalidation event to other instances
    Publish(ctx context.Context, event InvalidationEvent) error

    // Channel returns a channel that receives invalidation events from other instances
    Channel() <-chan *InvalidationEvent

    // Close releases resources
    Close() error
}
```

### Example: In-Memory Broker (for testing)

```go
runner := vrun.New()
defer runner.Stop()

broker := inmemory.New()
vlru.StartEventBroker(runner, broker)
```

## Key Serialization

Keys must be serializable for network transport. By default, the following key types are supported without configuration:

- `string` - zero-copy serialization
- `int` - converted via `strconv`
- `int64` - converted via `strconv`
- Other types - JSON encoding (fallback)

Built-in serializers for explicit configuration:

- `StringKeySerializer` - for string keys (zero-copy)
- `JSONKeySerializer[K]` - for any JSON-serializable key type

Custom serializers implement `KeySerializer[K]`:

```go
type KeySerializer[K comparable] interface {
    Serialize(key K) (string, error)
    Deserialize(s string) (K, error)
}
```

## Event Publishing Scope

Events are published for:
- Explicit `Remove()` calls
- LRU capacity evictions (when cache is full)
- TTL expirations (expirable variant)

Events are NOT published for:
- `Add()` operations (just local)
- `Purge()` operations (just local)
- Remote invalidations (prevents cascade)

## License

Apache 2.0
