package validators

type Validator struct {
	Errors map[string]interface{}
}

func NewValidator() *Validator {
	return &Validator{Errors: make(map[string]interface{})}
}

func (v *Validator) HasErrors() bool {
	return len(v.Errors) > 0
}

func (v *Validator) Check(ok bool, key, message string) {
	if !ok {
		v.AddError(key, message)
	}
}

// CheckError is a convenience method for checking if an error is nil
func (v *Validator) CheckError(err error, key, message string) {
	if err != nil && message == "" {
		message = err.Error()
	}
	v.Check(err == nil, key, message)
}

func (v *Validator) AddError(key, message string) {
	v.Errors[key] = message
}
