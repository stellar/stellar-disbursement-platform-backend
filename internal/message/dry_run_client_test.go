package message

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_DryRunClient(t *testing.T) {
	cc, _ := NewDryRunClient()

	// Email
	stdOut := os.Stdout

	r, w, err := os.Pipe()
	require.NoError(t, err)

	os.Stdout = w

	msg := Message{
		ToPhoneNumber: "",
		ToEmail:       "email@email.com",
		Title:         "My Message Title",
		Message:       "My email content",
	}
	err = cc.SendMessage(msg)
	require.NoError(t, err)

	w.Close()
	os.Stdout = stdOut

	buf := new(strings.Builder)
	_, err = io.Copy(buf, r)
	require.NoError(t, err)

	expected := `-------------------------------------------------------------------------------
Recipient: email@email.com
Subject: My Message Title
Content: My email content
-------------------------------------------------------------------------------
`
	assert.Equal(t, expected, buf.String())

	// SMS
	stdOut = os.Stdout

	r, w, err = os.Pipe()
	require.NoError(t, err)

	os.Stdout = w

	msg = Message{
		ToPhoneNumber: "+11111111111",
		ToEmail:       "",
		Title:         "My Message Title",
		Message:       "My SMS content",
	}
	err = cc.SendMessage(msg)
	require.NoError(t, err)

	w.Close()
	os.Stdout = stdOut

	buf = new(strings.Builder)
	_, err = io.Copy(buf, r)
	require.NoError(t, err)

	expected = `-------------------------------------------------------------------------------
Recipient: +11111111111
Subject: My Message Title
Content: My SMS content
-------------------------------------------------------------------------------
`
	assert.Equal(t, expected, buf.String())
}
