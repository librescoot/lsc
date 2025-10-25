package redis

import (
	"context"
	"fmt"
	"log"
	"time"

	rdb "github.com/redis/go-redis/v9"
)

// Client is a wrapper around the go-redis client with common functionality
type Client struct {
	client *rdb.Client
	ctx    context.Context
	logger *log.Logger
}

// XMessage represents a message from a Redis stream
type XMessage = rdb.XMessage

// XStream represents a Redis stream
type XStream = rdb.XStream

// PubSub represents a Redis pub/sub subscription
type PubSub = rdb.PubSub

// NewClient creates a new Redis client instance
func NewClient(addr string) *Client {
	return &Client{
		client: rdb.NewClient(&rdb.Options{
			Addr:            addr,
			DB:              0, // use default DB
			DisableIndentity: true, // Disable client identity features for older Redis versions
		}),
		ctx:    context.Background(),
		logger: log.New(log.Writer(), "[Redis] ", log.LstdFlags),
	}
}

// SetLogger sets a custom logger (use io.Discard to disable logging)
func (c *Client) SetLogger(l *log.Logger) {
	c.logger = l
}

// Connect pings the Redis server to ensure connectivity
func (c *Client) Connect() error {
	ctx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
	defer cancel()

	if err := c.client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("failed to connect to Redis: %w", err)
	}
	// Disable verbose logging - just verify connection silently
	return nil
}

// Close closes the Redis client connection
func (c *Client) Close() error {
	return c.client.Close()
}

// GetClient returns the underlying redis client for advanced operations
func (c *Client) GetClient() *rdb.Client {
	return c.client
}

// HGet retrieves a field from a Redis hash
func (c *Client) HGet(key, field string) (string, error) {
	return c.client.HGet(c.ctx, key, field).Result()
}

// HGetWithContext retrieves a field from a Redis hash with context
func (c *Client) HGetWithContext(ctx context.Context, key, field string) (string, error) {
	return c.client.HGet(ctx, key, field).Result()
}

// HSet sets a field in a Redis hash
func (c *Client) HSet(key, field, value string) error {
	return c.client.HSet(c.ctx, key, field, value).Err()
}

// HSetWithContext sets a field in a Redis hash with context
func (c *Client) HSetWithContext(ctx context.Context, key, field, value string) error {
	return c.client.HSet(ctx, key, field, value).Err()
}

// HGetAll retrieves all fields and values from a Redis hash
func (c *Client) HGetAll(key string) (map[string]string, error) {
	return c.client.HGetAll(c.ctx, key).Result()
}

// HGetAllWithContext retrieves all fields and values from a Redis hash with context
func (c *Client) HGetAllWithContext(ctx context.Context, key string) (map[string]string, error) {
	return c.client.HGetAll(ctx, key).Result()
}

// LPush pushes a value onto the head of a list
func (c *Client) LPush(key, value string) error {
	return c.client.LPush(c.ctx, key, value).Err()
}

// LPushWithContext pushes a value onto the head of a list with context
func (c *Client) LPushWithContext(ctx context.Context, key, value string) error {
	return c.client.LPush(ctx, key, value).Err()
}

// SMembers retrieves all members of a set
func (c *Client) SMembers(key string) ([]string, error) {
	return c.client.SMembers(c.ctx, key).Result()
}

// SMembersWithContext retrieves all members of a set with context
func (c *Client) SMembersWithContext(ctx context.Context, key string) ([]string, error) {
	return c.client.SMembers(ctx, key).Result()
}

// Subscribe subscribes to one or more Redis pub/sub channels
func (c *Client) Subscribe(ctx context.Context, channels ...string) *PubSub {
	return c.client.Subscribe(ctx, channels...)
}

// Publish publishes a message to a Redis channel
func (c *Client) Publish(ctx context.Context, channel string, message interface{}) error {
	return c.client.Publish(ctx, channel, message).Err()
}

// XRead reads messages from Redis streams
func (c *Client) XRead(ctx context.Context, args *rdb.XReadArgs) ([]XStream, error) {
	return c.client.XRead(ctx, args).Result()
}

// XReadStreams reads from multiple streams starting from given IDs
func (c *Client) XReadStreams(ctx context.Context, streams ...string) ([]XStream, error) {
	return c.client.XRead(ctx, &rdb.XReadArgs{
		Streams: streams,
		Count:   0,
		Block:   0,
	}).Result()
}

// Pipeline creates a new pipeline for batching commands
func (c *Client) Pipeline() rdb.Pipeliner {
	return c.client.Pipeline()
}
