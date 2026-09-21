package db

import "testing"

func TestValidatorAllowsReadOnly(t *testing.T) {
	v := NewValidator(5000)
	allowed := []string{
		"SELECT * FROM Users",
		"SELECT * FROM public.users LIMIT 10",
		"  SELECT id, name FROM Users WHERE id = 1  ",
		"WITH cte AS (SELECT id FROM Users) SELECT * FROM cte",
		"SELECT /* inline */ id FROM Users -- trailing comment",
		"SELECT id\nFROM Users\nWHERE name = 'INSERT'", // keyword only inside a string literal
		"(SELECT id FROM A) UNION (SELECT id FROM B)",
		"SELECT COUNT(*) FROM Orders;",
	}
	for _, q := range allowed {
		if err := v.Validate(q); err != nil {
			t.Errorf("expected query to be allowed, got error: %q -> %v", q, err)
		}
	}
}

func TestValidatorBlocksDangerous(t *testing.T) {
	v := NewValidator(5000)
	blocked := []string{
		"INSERT INTO Users VALUES (1)",
		"UPDATE Users SET name = 'x'",
		"DELETE FROM Users",
		"DROP TABLE Users",
		"ALTER TABLE Users ADD col INT",
		"TRUNCATE TABLE Users",
		"CREATE TABLE T (id INT)",
		"SELECT * INTO NewTable FROM Users",
		"SELECT * FROM Users; DROP TABLE Users",
		"BEGIN TRAN",
		"GRANT SELECT ON Users TO public",
		"SELECT * FROM Users -- safe\n; DELETE FROM Users",
	}
	for _, q := range blocked {
		if err := v.Validate(q); err == nil {
			t.Errorf("expected query to be blocked, but it was allowed: %q", q)
		}
	}
}

func TestValidatorCommentHidingIsCaught(t *testing.T) {
	v := NewValidator(5000)
	q := "SELECT * FROM Users /* comment */ ; DROP TABLE Users"
	if err := v.Validate(q); err == nil {
		t.Errorf("expected chained DROP to be blocked")
	}
}

func TestValidatorLengthLimit(t *testing.T) {
	v := NewValidator(20)
	if err := v.Validate("SELECT * FROM AVeryLongTableNameThatExceedsLimit"); err == nil {
		t.Errorf("expected length limit to be enforced")
	}
}

func TestValidatorEmpty(t *testing.T) {
	v := NewValidator(5000)
	if err := v.Validate("   "); err == nil {
		t.Errorf("expected empty query to be rejected")
	}
}

func TestValidatorMustStartWithSelectOrWith(t *testing.T) {
	v := NewValidator(5000)
	if err := v.Validate("EXPLAIN SELECT 1"); err == nil {
		t.Errorf("expected non-SELECT/WITH prefix to be rejected")
	}
}
