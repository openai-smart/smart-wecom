package smart_wecom

import (
	"context"
	"fmt"
	sc "github.com/openai-smart/smart-chat"
	"github.com/pkg/errors"
	"github.com/redis/go-redis/v9"
	"github.com/vmihailenco/msgpack/v5"
	"strconv"
	"time"
)

func today() string {
	return time.Now().Format("20060102")
}

var ctx = context.Background()

type Redis struct {
	sc.Cache

	client *redis.Client
}

func NewRedis(Addr string, Password string, db int) sc.Cache {
	return &Redis{client: redis.NewClient(&redis.Options{
		Addr:     Addr,
		Password: Password, // no password set
		DB:       db,       // use default DB
	})}
}

func (r Redis) Error(id sc.SessionID) (sc.SessionError, error) {
	//TODO implement me
	panic("implement me")
}

func (r Redis) ErrorStore(id sc.SessionID, sessionError sc.SessionError) error {
	// key -> error:[SessionID] => SessionError
	//TODO implement me
	panic("implement me")
}

func (r Redis) SessionStore(session *sc.Session) (err error) {
	// key -> used:[time]:[UserUID] => int
	// key -> session:[time]:[UserUID]:[SessionID] => Session

	r.client.Incr(ctx, fmt.Sprintf("used:%s:%s", today(), session.User.UID)) // 当天调用数+1

	if sessionPack, err := msgpack.Marshal(session); err == nil {
		// 保存session记录
		r.client.Set(ctx, fmt.Sprintf("session:%s:%s:%s", today(), session.User.UID, session.ID),
			sessionPack, -1)
	}
	return err
}

func (r Redis) Session(sessionID sc.SessionID) (session *sc.Session, err error) {
	// key -> session:record:[time]:[UserUID]:[SessionID] => Session
	if result, _, err := r.client.Scan(ctx, 0,
		fmt.Sprintf("session:record:*:%s", sessionID), 1).Result(); err == nil || result != nil {
		if err = msgpack.Unmarshal([]byte(result[0]), &session); err == nil {
			return session, nil
		}
	}

	return nil, err
}

func (r Redis) SessionStatusRecord(sessionID sc.SessionID, status sc.SessionStatus) error {
	// key -> session:status:[SessionID] -> SessionStatus
	_, err := r.client.Set(ctx, fmt.Sprintf("session:status:*:%s", sessionID), status, 0).Result()
	return err
}

func (r Redis) SessionStatus(sessionID sc.SessionID) (status sc.SessionStatus, err error) {
	// key -> session:status:[SessionID] -> SessionStatus
	if result, err := r.client.Get(ctx, fmt.Sprintf("session:status:*:%s", sessionID)).Result(); err == nil {
		if len(result) > 0 {
			if atoi, err := strconv.Atoi(result); err == nil {
				return sc.SessionStatus(atoi), nil
			}

		}
	}
	return sc.SessionStatusNone, err

}

func (r Redis) SessionHistory(userUID sc.UserUID, start time.Time, end time.Time) (sessions []sc.Session, err error) {
	// key -> session:record:[time]:[UserUID]:[SessionID] => Session

	// TODO 暂时返回全部记录
	if result, _, err := r.client.Scan(ctx, 0,
		fmt.Sprintf("session:record:*:%s*", userUID), 1).Result(); err == nil || result != nil {
		for i := range result {
			session := sc.Session{}
			if err = msgpack.Unmarshal([]byte(result[i]), &session); err == nil {
				sessions = append(sessions, session)
			}
		}
	}
	return sessions, err
}

func (r Redis) User(userUID sc.UserUID) (user *sc.User, err error) {
	// key -> user:info:[userUID] => User
	if result, err := r.client.Get(ctx, fmt.Sprintf("user:info:%s", userUID)).Result(); err == nil {
		if len(result) != 0 {
			if err = msgpack.Unmarshal([]byte(result), &user); err == nil {
				return user, nil
			}
		}
	}

	return nil, err
}

func (r Redis) UserID2UID(userID string) (userUID sc.UserUID, err error) {
	// key -> user:uid:[userID] => userUID
	result, err := r.client.Get(ctx,
		fmt.Sprintf("user:uid:%s", userID)).Result()
	if len(result) == 0 {
		return userUID, errors.New(fmt.Sprintf("userID[%s] not found", userID))
	}
	return sc.UserUID(result), err
}

func (r Redis) UserUIDBind(userID string, userUID sc.UserUID) (err error) {
	// key -> user:uid:[userID] => userUID

	// 检查用户是否存在
	user, err := r.User(userUID)
	if err != nil {
		return err
	}

	if user == nil {
		return errors.New(fmt.Sprintf("userUID[%s] not found", userUID))
	}

	if _, err = r.client.Set(ctx, fmt.Sprintf("user:uid:%s", userID), string(userUID), 0).Result(); err != nil {
		return err
	}

	return err
}

func (r Redis) UserStore(user *sc.User) (err error) {
	// key -> user:info:[userUID] => User
	if userPack, err := msgpack.Marshal(user); err == nil {
		_, err = r.client.Set(ctx, fmt.Sprintf("user:info:%s", user.UID), userPack, 0).Result()
	}
	return err
}

func (r Redis) UserSmartsStore(userUID sc.UserUID, smartIDs ...string) error {
	//key -> user:smart:[UserUID] > [...SmartID]
	_, err := r.client.SAdd(ctx, fmt.Sprintf("user:smart:%s", userUID), smartIDs).Result()
	return err
}

func (r Redis) UserSmartUIDs(userUID sc.UserUID) ([]string, error) {
	//key -> user:smart:[UserUID] > [...SmartID]
	return r.client.SMembers(ctx, fmt.Sprintf("user:smart:%s", userUID)).Result()
}

func (r Redis) UserAnswer(userUID sc.UserUID) ([]string, error) {
	return r.client.SMembers(ctx, fmt.Sprintf("user:question:%s", userUID)).Result()
}

func (r Redis) UserAnswerStore(userUID sc.UserUID, smartIDs ...string) (err error) {
	//key -> user:question:[UserUID] > [...SmartID]

	// 检查用户是否拥有 smart 权限
	for i := range smartIDs {
		exist, err := r.client.SIsMember(ctx, fmt.Sprintf("user:smart:%s", userUID), smartIDs[i]).Result()
		if err != nil {
			return err
		}
		if !exist {
			// 无权访问
			return errors.New(fmt.Sprintf("user[%s] access denied [%s]", userUID, smartIDs[i]))
		}
	}

	_, err = r.client.SAdd(ctx, fmt.Sprintf("user:question:%s", userUID), smartIDs).Result()

	return err
}

func (r Redis) UserBalance(id sc.UserUID) (float32, error) {
	//TODO implement me
	panic("implement me")
}

func (r Redis) ConfigureStore(id string, configure sc.Configure) (err error) {
	//key -> configure:[configureID]
	if configurePack, err := msgpack.Marshal(configure); err == nil {
		_, err = r.client.Set(ctx, fmt.Sprintf("configure:%s", id), configurePack, 0).Result()
	}
	return err
}

func (r Redis) Configure(id string) (configure sc.Configure, err error) {
	//key -> configure:[configureID]
	if result, err := r.client.Get(ctx, fmt.Sprintf("configure:%s", id)).Result(); err == nil {
		if len(result) > 0 {
			if err = msgpack.Unmarshal([]byte(result), &configure); err == nil {
				return configure, nil
			}
		}
	}

	return nil, err
}
