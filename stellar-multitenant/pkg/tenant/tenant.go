package tenant

type Tenant struct {
	ID   string `db:"id"`
	Name string `db:"name"`
}
