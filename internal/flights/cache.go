package flights

import (
	"context"
	"github.com/redis/go-redis/v9"
	"log"
	"time"
)

// Cache - Redis-кеш Flight Service (Cache-Aside), клиент - redis.UniversalClient, поддерживает как обычный Redis, так и Sentinel (failover)
type Cache struct {
	client redis.UniversalClient
	ttl    time.Duration
}

// NewStandaloneCache - одиночный Redis (для локального запуска без compose
func NewStandaloneCache(addr string, ttl time.Duration) *Cache {
	return &Cache{client: redis.NewClient(&redis.Options{Addr: addr}), ttl: ttl}
}

// NewFailoverCache - Sentinel-клиент: адрес текущего master запрашивается у sentinels и автоматически обновляется при переключении мастера
func NewFailoverCache(masterName string, sentinelAddrs []string, ttl time.Duration) *Cache {
	return &Cache{
		client: redis.NewFailoverClient(&redis.FailoverOptions{
			MasterName:    masterName,
			SentinelAddrs: sentinelAddrs,
		}),
		ttl: ttl,
	}
}

func (c *Cache) Close() error {
	return c.client.Close()
}

// Get возвращает (payload, true) при hit, при miss или недоступном Redis и запрос уходит в БД
func (c *Cache) Get(ctx context.Context, key string) ([]byte, bool) {
	payload, err := c.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		log.Printf("cache MISS key=%s", key)
		return nil, false
	}
	if err != nil {
		log.Printf("cache error key=%s: %v", key, err)
		return nil, false
	}
	log.Printf("cache HIT key=%s", key)
	return payload, true
}

// Set сохраняет payload в кеш с TTL
func (c *Cache) Set(ctx context.Context, key string, payload []byte) {
	if err := c.client.Set(ctx, key, payload, c.ttl).Err(); err != nil {
		log.Printf("cache set error key=%s: %v", key, err)
	}
}

// InvalidateFlight сбрасывает flight:{id} и все search:* кешей после мутаций
func (c *Cache) InvalidateFlight(ctx context.Context, flightID string) {
	if err := c.client.Del(ctx, "flight:"+flightID).Err(); err != nil {
		log.Printf("cache invalidate error flight:%s: %v", flightID, err)
	}
	searchKeys, err := c.client.Keys(ctx, "search:*").Result()
	if err != nil {
		log.Printf("cache invalidate search error: %v", err)
		return
	}
	if len(searchKeys) != 0 {
		if err = c.client.Del(ctx, searchKeys...).Err(); err != nil {
			log.Printf("cache invalidate search error: %v", err)
		}
	}
	log.Printf("cache INVALIDATED: flight:%s (+%d search keys)", flightID, len(searchKeys))
}
