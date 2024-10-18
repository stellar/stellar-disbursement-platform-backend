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
	// Define the function map that will be available inside the templates
	funcMap := template.FuncMap{
		"EmailStyle": func() template.HTML {
			return emailStyle
		},
	}

	// Parse the templates with the function map
	t, err := template.New("").Funcs(funcMap).ParseFS(Tmpl, "tmpl/*.tmpl", "tmpl/**/*.tmpl")
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

// emailStyle is the CSS style that will be included in the email templates.
const emailStyle = template.HTML(`
    <style>
        body {
			font-family: Arial, sans-serif;
			line-height: 1.6;
			color: #000000;
			background-color: #ffffff;
			margin: 0;
			padding: 20px;
		}
		p {
			margin-bottom: 16px;
		}
		.button {
			display: inline-block;
			padding: 10px 20px;
			background-color: #000000;
			color: #ffffff;
			text-decoration: none;
			border-radius: 5px;
			font-weight: bold;
		}
		.button:hover {
			background-color: #333333;
		}
		strong:hover {
			font-weight: bold;
			color: #cccccc;
		}
    </style>
`)
