package cahtgpt

import (
	"context"
	sc "github.com/openai-smart/smart-chat"
	"github.com/openai-smart/smart-chat/smart"
	"github.com/sashabaranov/go-openai"
)

type ChatGPT struct {
	smart.Smart

	client *openai.Client
}

func NewChatGPT(authToken string) smart.Smart {
	return &ChatGPT{
		client: openai.NewClient(authToken),
	}
}

func (chatgpt *ChatGPT) Platform() string {
	return "ChatGPT"
}

func (chatgpt *ChatGPT) Ask(q sc.Question) (sc.Answer, error) {
	resp, err := chatgpt.client.CreateChatCompletion(
		context.Background(),
		openai.ChatCompletionRequest{
			Model: openai.GPT3Dot5Turbo,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: q.(string),
				},
			},
		},
	)

	if err != nil {
		return nil, err
	}

	answer := resp.Choices[0].Message.Content

	return answer, nil
}

func (chatgpt *ChatGPT) Balance() (float32, error) {
	return 0, nil
}
