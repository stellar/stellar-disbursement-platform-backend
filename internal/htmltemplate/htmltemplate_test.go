package htmltemplate

import (
	"crypto/rand"
	"fmt"
	"html/template"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ExecuteHTMLTemplate(t *testing.T) {
	// File not found
	var inputData interface{}
	templateStr, err := ExecuteHTMLTemplate("non-existing-file.html", inputData)
	require.Empty(t, templateStr)
	require.EqualError(t, err, `executing html template: html/template: "non-existing-file.html" is undefined`)

	// handle invalid struct body
	inputData = struct {
		WrongFieldName string
	}{
		WrongFieldName: "foo bar",
	}
	templateStr, err = ExecuteHTMLTemplate("empty_body.tmpl", inputData)
	require.Empty(t, templateStr)
	require.EqualError(t, err, `executing html template: template: empty_body.tmpl:9:2: executing "empty_body.tmpl" at <.Body>: can't evaluate field Body in type struct { WrongFieldName string }`)

	// Success 🎉
	inputData = EmptyBodyEmailTemplate{Body: "foo bar"}

	templateStr, err = ExecuteHTMLTemplate("empty_body.tmpl", inputData)
	require.NoError(t, err)
	require.Contains(t, templateStr, "<body>\nfoo bar\n</body>")
}

func Test_ExecuteHTMLTemplateForEmailEmptyBody(t *testing.T) {
	// create a random string:
	randReader := rand.Reader
	b := make([]byte, 10)
	_, err := randReader.Read(b)
	require.NoError(t, err)
	randomStr := fmt.Sprintf("%x", b)[:10]

	// check if the random string is imprinted in the template
	inputData := EmptyBodyEmailTemplate{Body: template.HTML(randomStr)}
	templateStr, err := ExecuteHTMLTemplateForEmailEmptyBody(inputData)
	require.NoError(t, err)
	require.Contains(t, templateStr, randomStr)
}

func Test_ExecuteHTMLTemplateForStaffInvitationEmailMessage(t *testing.T) {
	forgotPasswordLink := "https://sdp.com/forgot-password"

	data := StaffInvitationEmailMessageTemplate{
		FirstName:          "First",
		Role:               "developer",
		ForgotPasswordLink: forgotPasswordLink,
		OrganizationName:   "Organization Name",
	}
	content, err := ExecuteHTMLTemplateForStaffInvitationEmailMessage(data)
	require.NoError(t, err)

	assert.Contains(t, content, "Hello, First!")
	assert.Contains(t, content, "as a developer.")
	assert.Contains(t, content, forgotPasswordLink)
	assert.Contains(t, content, "Organization Name")
}

func Test_ExecuteHTMLTemplateForStaffInvitationEmailMessage_HTMLInjectionAttack(t *testing.T) {
	forgotPasswordLink := "https://sdp.com/forgot-password"

	data := StaffInvitationEmailMessageTemplate{
		FirstName:          "First",
		Role:               "developer",
		ForgotPasswordLink: forgotPasswordLink,
		OrganizationName:   "<a href='evil.com'>Redeem funds</a>",
	}
	content, err := ExecuteHTMLTemplateForStaffInvitationEmailMessage(data)
	require.NoError(t, err)

	assert.Contains(t, content, "Hello, First!")
	assert.Contains(t, content, "as a developer.")
	assert.Contains(t, content, forgotPasswordLink)
	assert.Contains(t, content, "&lt;a href=&#39;evil.com&#39;&gt;Redeem funds&lt;/a&gt;")
}

func Test_ExecuteHTMLTemplateForStaffForgotPasswordEmailMessage(t *testing.T) {
	data := StaffForgotPasswordEmailMessageTemplate{
		ResetToken:        "resetToken",
		ResetPasswordLink: "https://sdp.com/reset-password",
		OrganizationName:  "Organization Name",
	}
	content, err := ExecuteHTMLTemplateForStaffForgotPasswordEmailMessage(data)
	require.NoError(t, err)

	assert.Contains(t, content, "<strong>resetToken</strong>")
	assert.Contains(t, content, "<a href=\"https://sdp.com/reset-password\">reset password page</a>")
	assert.Contains(t, content, "Organization Name")
}
