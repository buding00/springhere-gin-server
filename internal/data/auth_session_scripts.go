package data

import "github.com/redis/go-redis/v9"

// rotateSessionScript 用 CAS 轮换 Refresh，同一颗 Token 只能成功一次。
var rotateSessionScript = redis.NewScript(`
if redis.call('GET', KEYS[2]) ~= ARGV[3] then return 0 end
if redis.call('GET', KEYS[1]) ~= ARGV[1] then return 0 end
local ttl = redis.call('PTTL', KEYS[1])
if ttl <= 0 then return 0 end
redis.call('SET', KEYS[1], ARGV[2], 'PX', ttl)
redis.call('DEL', KEYS[2])
redis.call('SET', KEYS[3], ARGV[3], 'PX', ttl)
return ttl`)

// deleteSessionScript 删除会话时一并清掉 Refresh 索引和用户会话集合。
var deleteSessionScript = redis.NewScript(`
local raw = redis.call('GET', KEYS[1])
if not raw then return 0 end
local decoded, session = pcall(cjson.decode, raw)
redis.call('DEL', KEYS[1])
if decoded and session.refresh_digest then redis.call('DEL', ARGV[2] .. session.refresh_digest) end
if decoded and session.user_id then redis.call('ZREM', ARGV[3] .. session.user_id, ARGV[1]) end
return 1`)

// deleteUserSessionsScript 一次性删掉某用户的全部会话和 Refresh 索引。
var deleteUserSessionsScript = redis.NewScript(`
local sessionIDs = redis.call('ZRANGE', KEYS[1], 0, -1)
for _, sessionID in ipairs(sessionIDs) do
  local sessionKey = ARGV[1] .. sessionID
  local raw = redis.call('GET', sessionKey)
  if raw then
    local decoded, session = pcall(cjson.decode, raw)
    if decoded and session.refresh_digest then redis.call('DEL', ARGV[2] .. session.refresh_digest) end
    redis.call('DEL', sessionKey)
  end
end
redis.call('DEL', KEYS[1])
return #sessionIDs`)
