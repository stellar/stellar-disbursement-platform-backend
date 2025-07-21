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
		ResetPasswordLink: "https://sdp.com/reset-password?token=resetToken",
		OrganizationName:  "Organization Name",
	}
	content, err := ExecuteHTMLTemplateForStaffForgotPasswordEmailMessage(data)
	require.NoError(t, err)

	assert.Contains(t, content, `<a href="https://sdp.com/reset-password?token=resetToken" class="button">Reset Password</a>`)
	assert.Contains(t, content, "Organization Name")
}

func Test_ExecuteCustomHTMLTemplate(t *testing.T) {
	templateData := map[string]string{
		"OrganizationName": "Test Organization",
		"RegistrationLink": "https://example.com/register?token=abc123",
	}
	customTemplate := `<html><head><title>Welcome</title></head><body>{{EmailStyle}}<h1>Hello from {{.OrganizationName}}!</h1><p>Click <a href="{{.RegistrationLink}}">here</a> to register.</p></body></html>`

	result, err := ExecuteCustomHTMLTemplate(customTemplate, templateData)
	require.NoError(t, err)
	assert.Contains(t, result, "Test Organization")
	assert.Contains(t, result, "https://example.com/register?token=abc123")
	assert.Contains(t, result, "<style>")
	assert.Contains(t, result, ".button {")
}

func Test_ExecuteCustomHTMLTemplate_WithCustomCSS(t *testing.T) {
	templateData := map[string]string{
		"OrganizationName": "Test Organization",
		"RegistrationLink": "https://example.com/register?token=abc123",
	}
	customTemplate := `<html><head><title>Custom Styled Email</title><style>body { background: red; }.custom { color: blue; }</style></head><body>{{EmailStyle}}<h1>Hello from {{.OrganizationName}}!</h1><p class="custom">Click <a href="{{.RegistrationLink}}">here</a> to register.</p></body></html>`

	result, err := ExecuteCustomHTMLTemplate(customTemplate, templateData)
	require.NoError(t, err)
	assert.Contains(t, result, "Test Organization")
	assert.Contains(t, result, "https://example.com/register?token=abc123")
	assert.Contains(t, result, "background: red")
	assert.Contains(t, result, "color: blue")
	assert.NotContains(t, result, ".button {")
}

func Test_ExecuteCustomHTMLTemplate_EmptyContent(t *testing.T) {
	templateData := map[string]string{
		"OrganizationName": "Test Organization",
		"RegistrationLink": "https://example.com/register?token=abc123",
	}

	result, err := ExecuteCustomHTMLTemplate("", templateData)
	require.Empty(t, result)
	require.EqualError(t, err, "template content cannot be empty")
}

func Test_ExecuteCustomHTMLTemplate_InvalidSyntax(t *testing.T) {
	templateData := map[string]string{
		"OrganizationName": "Test Organization",
		"RegistrationLink": "https://example.com/register?token=abc123",
	}
	invalidTemplate := `<html><body>Hello {{.OrganizationName</body></html>`

	result, err := ExecuteCustomHTMLTemplate(invalidTemplate, templateData)
	require.Empty(t, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "error parsing custom template")
}

func Test_ExecuteCustomHTMLTemplate_HTMLInjection(t *testing.T) {
	maliciousData := map[string]string{
		"OrganizationName": "<script>alert('xss')</script>Malicious Org",
		"RegistrationLink": "javascript:alert('xss')",
	}
	simpleTemplate := `<html><body><h1>{{.OrganizationName}}</h1><a href="{{.RegistrationLink}}">Link</a></body></html>`

	result, err := ExecuteCustomHTMLTemplate(simpleTemplate, maliciousData)
	require.NoError(t, err)

	assert.Contains(t, result, "&lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt;") // Script tag is escaped
	assert.NotContains(t, result, "<script>alert('xss')</script>")
	assert.Contains(t, result, "#ZgotmplZ") // Go template marker for sanitized content
	assert.NotContains(t, result, "javascript:alert")
}

func Test_ProcessSubjectTemplate(t *testing.T) {
	templateData := map[string]string{
		"OrganizationName": "Test Organization",
		"RegistrationLink": "https://example.com/register?token=abc123",
	}
	subjectTemplate := "Payment from {{.OrganizationName}} - Action Required"

	result, err := ProcessSubjectTemplate(subjectTemplate, templateData)
	require.NoError(t, err)
	assert.Equal(t, "Payment from Test Organization - Action Required", result)
}

func Test_ProcessSubjectTemplate_EmptyTemplate(t *testing.T) {
	templateData := map[string]string{
		"OrganizationName": "Test Organization",
		"RegistrationLink": "https://example.com/register?token=abc123",
	}

	result, err := ProcessSubjectTemplate("", templateData)
	require.NoError(t, err)
	assert.Equal(t, "", result)
}

func Test_ProcessSubjectTemplate_MultipleVariables(t *testing.T) {
	templateData := map[string]string{
		"OrganizationName": "Test Organization",
		"RegistrationLink": "https://example.com/register?token=abc123",
	}
	subjectTemplate := "{{.OrganizationName}}: Complete registration at {{.RegistrationLink}}"

	result, err := ProcessSubjectTemplate(subjectTemplate, templateData)
	require.NoError(t, err)
	assert.Equal(t, "Test Organization: Complete registration at https://example.com/register?token=abc123", result)
}

func Test_ProcessSubjectTemplate_InvalidSyntax(t *testing.T) {
	templateData := map[string]string{
		"OrganizationName": "Test Organization",
		"RegistrationLink": "https://example.com/register?token=abc123",
	}
	invalidTemplate := "Payment from {{.OrganizationName - Action Required"

	result, err := ProcessSubjectTemplate(invalidTemplate, templateData)
	require.Empty(t, result)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "error parsing subject template")
}
