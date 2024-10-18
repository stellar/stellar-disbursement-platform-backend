package htmltemplate

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
)

//go:embed tmpl/**/*.tmpl "tmpl/*.tmpl"
var Tmpl embed.FS

func ExecuteHTMLTemplate(templateName string, data interface{}) (string, error) {
	t, err := template.ParseFS(Tmpl, "tmpl/*.tmpl", "tmpl/**/*.tmpl")
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
	Body template.HTML
}

func ExecuteHTMLTemplateForEmailEmptyBody(data EmptyBodyEmailTemplate) (string, error) {
	return ExecuteHTMLTemplate("empty_body.tmpl", data)
}

type StaffInvitationEmailMessageTemplate struct {
	FirstName          string
	Role               string
	ForgotPasswordLink string
	OrganizationName   string
}

func ExecuteHTMLTemplateForStaffInvitationEmailMessage(data StaffInvitationEmailMessageTemplate) (string, error) {
	return ExecuteHTMLTemplate("staff_invitation_message.tmpl", data)
}

type ForgotPasswordEmailMessageTemplate struct {
	ResetToken        string
	ResetPasswordLink string
	OrganizationName  string
}

func ExecuteHTMLTemplateForForgotPasswordEmailMessage(data ForgotPasswordEmailMessageTemplate) (string, error) {
	return ExecuteHTMLTemplate("staff_forgot_password_message.tmpl", data)
}

type StaffMFAEmailMessageTemplate struct {
	MFACode          string
	OrganizationName string
}

func ExecuteHTMLTemplateForStaffMFAEmailMessage(data StaffMFAEmailMessageTemplate) (string, error) {
	return ExecuteHTMLTemplate("staff_mfa_message.tmpl", data)
}
