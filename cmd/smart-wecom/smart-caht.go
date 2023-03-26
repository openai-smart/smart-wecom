package main

import (
	"fmt"
	sc "github.com/openai-smart/smart-chat"
	"github.com/openai-smart/smart-wecom/cmd"
	"github.com/rs/zerolog/log"
)

func main() {

	cli := cmd.Cli{}
	cli.SetRedis("127.0.0.1:6379", "123456")

	chatGPTConfigureID := cli.AddChatGPTConfigure("sk-sxxxxxxxxxxxxxxxxxxxxxxxxxxxxx") // 将chatGPT配置导入到数据库，导入后注释此段代码

	wecomConfigureID := cli.AddWecomConfigure(sc.Configure{ // 将企微配置导入到数据库，导入后注释此段代码
		"corpID":     "xxxxxxxxxx",
		"corpSecret": "xxxxxxxxxxxxxxxxxxxx-xxxx",
		"agentID":    1000005,
		"evens": []map[string]string{
			{
				"uri":            "/api/v1/chatgpt",
				"token":          "xxxxxxxx",
				"encodingAESKey": "xxxxxxxxxxxxxxxxxxx",
			},
		},
	})

	fmt.Println(wecomConfigureID, chatGPTConfigureID)
	c := cli.NewChat(wecomConfigureID, chatGPTConfigureID)
	cli.ConfigureWecomUsers(chatGPTConfigureID) // 第一次执行需要导入企微用户，导入后注释此段代码
	// 等待消息
	if err := c.Accept("[::]:8002"); err != nil {
		log.Fatal().Msg(err.Error())
	}
}
