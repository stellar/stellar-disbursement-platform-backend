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

type StaffForgotPasswordEmailMessageTemplate struct {
	ResetPasswordLink string
	OrganizationName  string
}

func ExecuteHTMLTemplateForStaffForgotPasswordEmailMessage(data StaffForgotPasswordEmailMessageTemplate) (string, error) {
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
			color: #333333;
			background-color: #ffffff;
			margin: 0;
			padding: 40px;
			max-width: 600px;
			margin: 0 auto;
		}
		p {
			margin-bottom: 20px;
			font-size: 16px;
		}
		.button {
			display: inline-block;
			padding: 12px 24px;
			background-color: #2c2c2c;
			color: #ffffff !important;
			text-decoration: none;
			border-radius: 6px;
			font-weight: 500;
			margin: 12px 0;
		}
		.button:hover {
			background-color: #404040;
		}
		/* Email client overrides */
		a.button {
			color: #ffffff !important;
		}
		a.button:link {
			color: #ffffff !important;
		}
		a.button:visited {
			color: #ffffff !important;
		}
		a.button:hover {
			color: #ffffff !important;
		}
		a.button:active {
			color: #ffffff !important;
		}
    </style>
`)
