package tencent

import (
	"fmt"
	sc "github.com/openai-smart/smart-chat"
	"github.com/openai-smart/smart-chat/chat"
	"github.com/openai-smart/smart-chat/smart"
	"github.com/rs/zerolog/log"
	"github.com/xen0n/go-workwx"
	"net/http"
	"time"
)

// CorpID 企微ID
type CorpID string

// WecomAppConfigure 企微应用配置
type WecomAppConfigure struct {
	CorpSecret string
	AgentID    int64
}

// WecomAppEventConfigure 企微接收消息服务器配置
type WecomAppEventConfigure struct {
	Uri            string
	Token          string
	EncodingAESKey string
}

// WecomApp 企微应用APP
type WecomApp struct {
	client *workwx.WorkwxApp
}

// NewWecomChatApp 创建一个APP聊天客户端
func NewWecomChatApp(wx *workwx.Workwx, configure *WecomAppConfigure) *WecomApp {
	client := wx.WithApp(configure.CorpSecret, configure.AgentID)
	client.SpawnAccessTokenRefresher()

	return &WecomApp{
		client: client,
	}
}

// ChatGPTCompletionHandler 发送消息到企微
func (app *WecomApp) ChatGPTCompletionHandler(session *sc.Session) error {
	rxMsg := session.Question.(*workwx.RxMessage)
	// TODO 内容大小判断，超出则分批发送
	recipient := workwx.Recipient{
		UserIDs: []string{rxMsg.FromUserID},
	}

	return app.client.SendMarkdownMessage(&recipient, session.Answer.(string), false)
}

// ExportDepts 导出的所有部门成员信息，取决于app权限，这里默认只返回一个
func (app *WecomApp) ExportDepts() ([]*workwx.UserInfo, error) {
	depts, err := app.client.ListAllDepts()
	if err != nil {
		return nil, err
	}
	if len(depts) == 0 {
		return nil, nil
	}
	return app.client.ListUsersByDeptID(depts[0].ID, true)

}

type WecomAppChat struct {
	chat.Chat

	app     *WecomApp
	filters []chat.Filter
	mux     *http.ServeMux
	chs     []chat.CompletionHandler

	smart map[string]smart.Smart

	cache sc.Cache
}

func NewSmartWecomChat(app *WecomApp, smart map[string]smart.Smart,
	filter []chat.Filter, cache sc.Cache) chat.Chat {
	return &WecomAppChat{
		app:     app,
		mux:     http.NewServeMux(),
		smart:   smart,
		filters: filter,
		cache:   cache,
	}
}

func (wecomChat *WecomAppChat) Platform() string {
	return "wecom"
}

func (wecomChat *WecomAppChat) smartChatProcess(session sc.Session) {
	defer func() {
		err := recover()
		if err != nil {
			log.Error().Msg(fmt.Sprintf("[%s] %s", session.ID, err))
		}

	}()
	text, _ := session.Question.(*workwx.RxMessage).Text()
	answer, err := wecomChat.smart[session.SmartID].Ask(text.GetContent())
	if err != nil {
		log.Error().Msg(err.Error())
		return
	}

	session.Answer = answer

	for i := range wecomChat.chs {
		if err := wecomChat.chs[i](&session); err != nil {
			return // TODO CompletionHandler出现错误后面的不会继续执行，需要一个合理解决方法
		}
	}

	if err := wecomChat.cache.SessionStore(&session); err != nil {
		log.Error().Msg(fmt.Sprintf("[%s] store session error %s", session.ID, err.Error()))
		return
	}

}

// OnIncomingMessage 接收来自企微发送的消息
// https://developer.work.weixin.qq.com/document/path/90930
func (wecomChat *WecomAppChat) OnIncomingMessage(rxMsg *workwx.RxMessage) error {
	log.Debug().Msg("incoming message: " + rxMsg.String())

	// 企微用户ID转换为 UserUID 用户ID
	userUID, err := wecomChat.cache.UserID2UID(fmt.Sprintf("%s:%s", wecomChat.Platform(), rxMsg.FromUserID))

	// 企微消息ID转换为 SessionID 会话ID
	sessionID := sc.SessionID(fmt.Sprintf("%s:%d", wecomChat.Platform(), rxMsg.MsgID))

	if err != nil {
		log.Warn().Msg(fmt.Sprintf("[%s] wecomChat.cache.UserID2UID %s", sessionID, err.Error()))
		return nil // TODO 出现错误是否需要企微补发消息？
	}

	// 找到发送用户的信息
	user, err := wecomChat.cache.User(userUID)
	if err != nil {
		log.Warn().Msg(fmt.Sprintf("[%s] wecomChat.cache.User %s", sessionID, err.Error()))
		return nil // TODO 出现错误是否需要企微补发消息？
	}
	if user == nil { // 用户未找到
		log.Warn().Msg(fmt.Sprintf("[%s] user [%s] not found %s", sessionID, userUID, err.Error()))
		return nil
	}

	smartIDs, _ := wecomChat.cache.UserAnswer(userUID)

	if err != nil {
		log.Warn().Msg(fmt.Sprintf("[%s] wecomChat.cache.User %s", sessionID, err.Error()))
		return nil // TODO 出现错误是否需要企微补发消息？
	}

	session := sc.Session{
		ID:       sessionID,
		User:     user,
		Question: rxMsg,
	}

	for i := range wecomChat.filters {
		if err := wecomChat.filters[i].DoFilter(&session); err != nil {
			log.Warn().Msg(err.Error())
			// TODO 处理被拦截的消息
			return nil
		}
	}

	// 开始向smart提问
	for i := range smartIDs {
		session.SmartID = smartIDs[i]
		go wecomChat.smartChatProcess(session)
		time.Sleep(500)
	}

	return nil
}

func (wecomChat *WecomAppChat) AddCompletionHandler(ch chat.CompletionHandler) {
	// TODO 同步锁
	wecomChat.chs = append(wecomChat.chs, ch)
}

// AddEventHandler 创建应用接收消息API处理器
func (wecomChat *WecomAppChat) AddEventHandler(ec chat.EventConfigure) error {
	configure := ec.(*WecomAppEventConfigure)
	handler, err := workwx.NewHTTPHandler(configure.Token, configure.EncodingAESKey, wecomChat)
	if err != nil {
		return err
	}
	wecomChat.mux.Handle(configure.Uri, handler)
	return nil
}

func (wecomChat *WecomAppChat) Accept(addr string) error {
	if err := http.ListenAndServe(addr, wecomChat.mux); err != nil {
		log.Fatal().Msg(err.Error())
		return err
	}

	return nil
}

func (wecomChat *WecomAppChat) Filters() []chat.Filter {
	return wecomChat.filters[:]
}

func (wecomChat *WecomAppChat) AddFilter(filter chat.Filter) {
	// TODO 同步锁
	wecomChat.filters = append(wecomChat.filters, filter)
}
