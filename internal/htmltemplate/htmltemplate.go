package htmltemplate

import (
	"bytes"
	"embed"
	"fmt"
	"text/template"
)

//go:embed tmpl/*.tmpl
var Tmpl embed.FS

func ExecuteHTMLTemplate(templateName string, data interface{}) (string, error) {
	t, err := template.ParseFS(Tmpl, "tmpl/*.tmpl")
	if err != nil {
		return "", fmt.Errorf("error parsing embedded template files: %w", err)
	}

	var executedTemplate bytes.Buffer
	err = t.ExecuteTemplate(&executedTemplate, templateName, data)
	if err != nil {
		return "", fmt.Errorf("executing html template: %w", err)
	}

	return executedTemplate.String(), nil
}

type EmptyBodyEmailTemplate struct {
	Body string
}

func ExecuteHTMLTemplateForEmailEmptyBody(data EmptyBodyEmailTemplate) (string, error) {
	return ExecuteHTMLTemplate("empty_body.tmpl", data)
}

type InvitationMessageTemplate struct {
	FirstName          string
	Role               string
	ForgotPasswordLink string
	OrganizationName   string
}

func ExecuteHTMLTemplateForInvitationMessage(data InvitationMessageTemplate) (string, error) {
	return ExecuteHTMLTemplate("invitation_message.tmpl", data)
}

type ForgotPasswordMessageTemplate struct {
	ResetToken        string
	ResetPasswordLink string
	OrganizationName  string
}

func ExecuteHTMLTemplateForForgotPasswordMessage(data ForgotPasswordMessageTemplate) (string, error) {
	return ExecuteHTMLTemplate("forgot_password_message.tmpl", data)
}

type MFAMessageTemplate struct {
	MFACode          string
	OrganizationName string
}

func ExecuteHTMLTemplateForMFAMessage(data MFAMessageTemplate) (string, error) {
	return ExecuteHTMLTemplate("mfa_message.tmpl", data)
}
