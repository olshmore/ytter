package email

import (
	"testing"

	"github.com/olshmore/ytter/pkg/config"
	"github.com/stretchr/testify/require"
)

func TestSendEmailWithGmail(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	config, err := config.LoadConfig("../../config")
	require.NoError(t, err)

	sender := NewGmailSender(config.EmailSenderName, config.EmailSenderAddress, config.EmailSenderPassword)

	subject := "A test email"
	content := `
	<h1>Hello</h1>
	<p>This is a test message from Ytter</p>
	`
	to := []string{"olyshsuk@gmail.com"}
	attachedFiles := []string{"../../README.md"}

	err = sender.SendEmail(subject, content, to, nil, nil, attachedFiles)
	require.NoError(t, err)
}
