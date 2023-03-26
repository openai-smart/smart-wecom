package cmd

import (
	"fmt"
	sc "github.com/openai-smart/smart-chat"
	"github.com/openai-smart/smart-chat/chat"
	"github.com/openai-smart/smart-chat/smart"
	sw "github.com/openai-smart/smart-wecom"
	cahtgpt "github.com/openai-smart/smart-wecom/chatgpt"
	"github.com/openai-smart/smart-wecom/tencent"
	"github.com/openai-smart/smart-wecom/tencent/filter"
	"github.com/openai-smart/smart-wecom/utils"
	"github.com/pkg/errors"
	"github.com/rs/zerolog/log"
	"github.com/xen0n/go-workwx"
)

type Cli struct {
	cache    sc.Cache
	wecomApp *tencent.WecomApp
	chat     chat.Chat
}

func (c *Cli) SetRedis(addr string, passwd string) {
	c.cache = sw.NewRedis(addr, passwd, 0)
}

func (c *Cli) addUser(user *sc.User) error {

	u, err := c.cache.User(user.UID)
	if err != nil {
		return err
	}

	if u != nil {
		return errors.New(fmt.Sprintf("user[%s] exists", user.UID))
	}

	return c.cache.UserStore(user)
}

// ConfigureWecomUsers 从部门导入企微用户，此APP而可见部门所有成员
// smarts 为用户配置聊天的smart
func (c *Cli) ConfigureWecomUsers(smarts ...string) {
	users, err := c.wecomApp.ExportDepts()
	if err != nil {
		log.Fatal().Msg(err.Error())
		return
	}

	for i := range users {
		user := &sc.User{
			UID:    sc.UserUID(utils.MD5(users[i].UserID)), // 全平台使用手机号MD5值作为唯一字段
			Name:   users[i].Name,
			Status: sc.UserStatusActive,
		}
		if err := c.addUser(user); err != nil {
			log.Fatal().Msg(fmt.Sprintf("[x] import user[%s] failed, %s", users[i].Name, err.Error()))
			continue
		}

		if err := c.cache.UserUIDBind(fmt.Sprintf("%s:%s", c.chat.Platform(), users[i].UserID), user.UID); err != nil {
			log.Fatal().Msg(fmt.Sprintf("[x] bind user[%s] uid failed, %s", users[i].Name, err.Error()))
		}
		log.Info().Msg(fmt.Sprintf("[*] import user[%s] success", users[i].Name))

		// 保存用户拥有的smart
		err := c.cache.UserSmartsStore(user.UID, smarts...)
		if err != nil {
			log.Fatal().Msg(fmt.Sprintf("[x] user[%s] smart[%s] store failed, %s", users[i].Name, smarts, err.Error()))
		}

		// 设置提问时使用哪个smart
		err = c.cache.UserAnswerStore(user.UID, smarts...)
		if err != nil {
			log.Fatal().Msg(fmt.Sprintf("[x] user[%s] answer store failed, %s", users[i].Name, err.Error()))
		}
		log.Info().Msg(fmt.Sprintf("[*] import user[%s] answer store success", users[i].Name))
	}

}

// addConfigure 新增配置信息
func (c *Cli) addConfigure(configureID string, configure sc.Configure) {
	err := c.cache.ConfigureStore(configureID, configure)
	if err != nil {
		log.Fatal().Msg(fmt.Sprintf("[x] import configure[%s] failed, %s", configureID, err.Error()))
		return
	}
	log.Info().Msg(fmt.Sprintf("[*] import configure[%s] suceess", configureID))
}

func (c *Cli) AddChatGPTConfigure(token string) string {
	configureID := fmt.Sprintf("chatgpt:%s", utils.MD5(token))
	c.addConfigure(configureID, sc.Configure{
		"token": token,
	})
	return configureID
}

func (c *Cli) NewSmart(configureID string) smart.Smart {
	configure, err := c.cache.Configure(configureID)
	if err != nil {
		log.Fatal().Msg(fmt.Sprintf("[x] new smart[%s] failed, %s", configureID, err.Error()))
	}
	return cahtgpt.NewChatGPT(configure["token"].(string))
}

func (c *Cli) AddWecomConfigure(configure sc.Configure) string {
	configure["agentID"] = int64(configure["agentID"].(int))
	configureID := fmt.Sprintf("wecom:%s",
		utils.MD5(fmt.Sprintf("%s%d", configure["corpID"], configure["agentID"])))

	c.addConfigure(configureID, configure)
	return configureID
}

func (c *Cli) NewChat(configureID string, smarts ...string) chat.Chat {
	configure, err := c.cache.Configure(configureID)
	if err != nil {
		log.Fatal().Msg(fmt.Sprintf("[x] new smart[%s] failed, %s", configureID, err.Error()))
	}

	var wecomClient = workwx.New(configure["corpID"].(string))

	corpSecret := configure["corpSecret"].(string)
	agentID := configure["agentID"].(int64)

	c.wecomApp = tencent.NewWecomChatApp(
		wecomClient,
		&tencent.WecomAppConfigure{
			CorpSecret: corpSecret,
			AgentID:    agentID,
		},
	)

	var chatGPTs = make(map[string]smart.Smart)

	for i := range smarts {
		chatGPTs[smarts[i]] = c.NewSmart(smarts[i])
	}

	c.chat = tencent.NewSmartWecomChat(
		c.wecomApp,
		chatGPTs, // 绑定已创建的AI
		[]chat.Filter{ // 拦截器，自定义拦截规则
			filter.NewDefaultFilter(c.cache),
		},
		c.cache,
	)

	c.chat.AddCompletionHandler(c.wecomApp.ChatGPTCompletionHandler)
	evens := configure["evens"].([]interface{})
	for i := range evens {
		// 创建一个监听事件接口，用于接收用户发送的消息
		if err := c.chat.AddEventHandler(&tencent.WecomAppEventConfigure{
			Uri:            evens[i].(map[string]interface{})["uri"].(string),
			Token:          evens[i].(map[string]interface{})["token"].(string),
			EncodingAESKey: evens[i].(map[string]interface{})["encodingAESKey"].(string),
		}); err != nil {
			log.Fatal().Msg(err.Error())
		}
	}

	return c.chat
}
