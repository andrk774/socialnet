package realtime

import (
	"context"
	"strconv"

	"socialnet/internal/redisx"
)

const (
	BusChannel = "sn:events" // канал событий; в него же пишет воркер
	instTTL    = 30          // секунд: экземпляр, не отметившийся дольше, считается упавшим
)

// RedisBus — шина на Redis pub/sub. Кто где подключён — множество
// sn:ws:<user> с id экземпляров; живость экземпляра — ключ sn:inst:<id> с TTL,
// так что соединения упавшего экземпляра «онлайн» не держат.
type RedisBus struct {
	R        *redisx.Client
	Instance string
}

func (b *RedisBus) Publish(ctx context.Context, payload []byte) error {
	_, err := b.R.Do(ctx, "PUBLISH", BusChannel, payload)
	return err
}

func (b *RedisBus) Subscribe(ctx context.Context) <-chan []byte {
	out := make(chan []byte, 256)
	msgs := b.R.Subscribe(ctx, BusChannel)
	go func() {
		defer close(out)
		for m := range msgs {
			out <- []byte(m.Payload)
		}
	}()
	return out
}

func wsKey(user int64) string { return "sn:ws:" + strconv.FormatInt(user, 10) }

func (b *RedisBus) SetOnline(ctx context.Context, user int64, online bool) error {
	if online {
		if _, err := b.R.Do(ctx, "SADD", wsKey(user), b.Instance); err != nil {
			return err
		}
		_, err := b.R.Do(ctx, "EXPIRE", wsKey(user), 86400)
		return err
	}
	_, err := b.R.Do(ctx, "SREM", wsKey(user), b.Instance)
	return err
}

// Одним запросом: есть ли пользователь на другом живом экземпляре.
const remoteOnlineScript = `
for _, id in ipairs(redis.call('SMEMBERS', KEYS[1])) do
  if id ~= ARGV[1] then
    if redis.call('EXISTS', 'sn:inst:' .. id) == 1 then return 1 end
    redis.call('SREM', KEYS[1], id)
  end
end
return 0`

func (b *RedisBus) RemoteOnline(ctx context.Context, user int64) bool {
	n, err := b.R.Int(ctx, "EVAL", remoteOnlineScript, 1, wsKey(user), b.Instance)
	return err == nil && n == 1
}

func (b *RedisBus) Heartbeat(ctx context.Context) error {
	_, err := b.R.Do(ctx, "SET", "sn:inst:"+b.Instance, "1", "EX", instTTL)
	return err
}
