package mail

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/smtp"
	"path/filepath"

	"github.com/evgeny/3d-maps/internal/config"
)

// Mailer — сервис для отправки почты.
type Mailer struct {
	cfg *config.Config
}

// NewMailer создаёт новый экземпляр Mailer.
func NewMailer(cfg *config.Config) *Mailer {
	return &Mailer{cfg: cfg}
}

// SendModelEmail отправляет 3D-модель пользователю.
func (m *Mailer) SendModelEmail(to, filename string, data []byte) error {
	if m.cfg.SMTPHost == "" {
		return fmt.Errorf("SMTP host not configured")
	}

	subject := "Ваша 3D-модель готова! 🏙️"
	body := "Добрый день!\n\nВаша 3D-модель города была успешно сгенерирована и приложена к этому письму.\n\nСпасибо, что пользуетесь нашим сервисом!"

	// Создаём MIME-сообщение
	buf := new(bytes.Buffer)
	writer := multipart.NewWriter(buf)

	// Заголовки письма
	fmt.Fprintf(buf, "From: %s\r\n", m.cfg.SMTPFrom)
	fmt.Fprintf(buf, "To: %s\r\n", to)
	fmt.Fprintf(buf, "Subject: %s\r\n", subject)
	fmt.Fprintf(buf, "MIME-Version: 1.0\r\n")
	fmt.Fprintf(buf, "Content-Type: multipart/mixed; boundary=%s\r\n", writer.Boundary())
	fmt.Fprintf(buf, "\r\n")

	// Текст письма
	textPart, _ := writer.CreatePart(map[string][]string{
		"Content-Type": {"text/plain; charset=utf-8"},
	})
	textPart.Write([]byte(body))

	// Вложение
	attachmentPart, _ := writer.CreatePart(map[string][]string{
		"Content-Type":              {http.DetectContentType(data)},
		"Content-Disposition":        {fmt.Sprintf("attachment; filename=\"%s\"", filepath.Base(filename))},
		"Content-Transfer-Encoding": {"base64"},
	})

	// Кодируем вложение в base64
	encoder := base64.NewEncoder(base64.StdEncoding, attachmentPart)
	encoder.Write(data)
	encoder.Close()

	writer.Close()

	// Отправка через SMTP
	auth := smtp.PlainAuth("", m.cfg.SMTPUser, m.cfg.SMTPPass, m.cfg.SMTPHost)
	addr := fmt.Sprintf("%s:%d", m.cfg.SMTPHost, m.cfg.SMTPPort)

	// В продакшене лучше использовать TLS, но для простоты начнём с обычного SMTP
	err := smtp.SendMail(addr, auth, m.cfg.SMTPFrom, []string{to}, buf.Bytes())
	if err != nil {
		return fmt.Errorf("send mail: %w", err)
	}

	return nil
}
