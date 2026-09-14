package utils

import (
	"fmt"
	"strings"

	"github.com/fast-template/springhere-gin-server/pkg/config"
	"github.com/google/uuid"
	"github.com/mojocn/base64Captcha"
)

const captchaSource = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"

// ImageCaptcha 生成字母数字图形验证码，存储由调用方负责。
type ImageCaptcha struct {
	driver *base64Captcha.DriverString
}

// NewImageCaptcha 按配置创建验证码画图器。
func NewImageCaptcha(cfg config.CaptchaConfig) *ImageCaptcha {
	driver := base64Captcha.NewDriverString(
		cfg.Height,
		cfg.Width,
		cfg.Length,
		base64Captcha.OptionShowHollowLine|base64Captcha.OptionShowSineLine,
		cfg.Length,
		captchaSource,
		nil,
		nil,
		nil,
	)
	return &ImageCaptcha{driver: driver}
}

// Generate 返回 UUID、PNG Data URL 和规范化答案。
func (g *ImageCaptcha) Generate() (id, image, answer string, err error) {
	if g == nil || g.driver == nil {
		return "", "", "", fmt.Errorf("captcha generator is not initialized")
	}
	_, content, answer := g.driver.GenerateIdQuestionAnswer()
	item, err := g.driver.DrawCaptcha(content)
	if err != nil {
		return "", "", "", fmt.Errorf("draw captcha: %w", err)
	}
	return uuid.NewString(), item.EncodeB64string(), strings.ToUpper(answer), nil
}
