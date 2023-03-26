package filter

import (
	"fmt"
	sc "github.com/openai-smart/smart-chat"
	"github.com/openai-smart/smart-chat/chat"
	"github.com/pkg/errors"
	"github.com/xen0n/go-workwx"
	"time"
)

type DefaultFilter struct {
	chat.Filter
	cache sc.Cache
}

func NewDefaultFilter(cache sc.Cache) chat.Filter {
	return &DefaultFilter{
		cache: cache,
	}
}

func (filter *DefaultFilter) DoFilter(session *sc.Session) (err error) {

	// 权限拦截器
	if err = filter.AccessFilter(session); err != nil {
		return err
	}

	// 消息拦截器
	if err = filter.IncomingMessageFilter(session); err != nil {
		return err
	}

	return nil
}

func (filter *DefaultFilter) IncomingMessageFilter(session *sc.Session) error {

	rxMsg := session.Question.(*workwx.RxMessage)
	msgType := coverWecomMsgType(rxMsg.MsgType)

	// 拦截超时消息
	t := time.Now().Sub(rxMsg.SendTime)
	if t > 10*time.Second {
		return errors.New(fmt.Sprintf("[%d] overtime %s", t, session.ID))
	}

	// 拦截重复消息
	status, err := filter.cache.SessionStatus(session.ID)
	if err != nil {
		return errors.New(err.Error())
	}

	if status == sc.SessionStatusProcessing || status == sc.SessionStatusCompletion {
		return errors.New(fmt.Sprintf("[%s] message status [%d] multiple sending ", session.ID, status))
	}

	// 拦截非MessageTypeText类型消息
	if msgType != sc.MessageTypeText {
		return errors.New(fmt.Sprintf("[%s] message type [%s] unsupported", session.ID, msgType))
	}

	return nil
}

// coverWecomMsgType TODO 实现其它消息类型转换
func coverWecomMsgType(msgType any) sc.MessageType {
	switch msgType.(workwx.MessageType) {
	case workwx.MessageTypeText:
		return sc.MessageTypeText
	default:
		return sc.MessageTypeUnknown
	}
}

func (filter *DefaultFilter) AccessFilter(session *sc.Session) error {

	// 权限检查
	if session.User == nil {
		return errors.New(fmt.Sprintf("[%s] Unauthorized", session.ID))
	}

	// 余额检查
	//balance, _ := filter.cache.UserBalance(session.User.ID)
	//if balance <= 0 {
	//	return errors.New(fmt.Sprintf("[%d] Sorry, your credit is running low", session.ID))
	//}
	// TODO 次数拦截器
	return nil
}
