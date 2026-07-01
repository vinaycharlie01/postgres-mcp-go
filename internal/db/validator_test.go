package db

import "testing"

func TestValidatorAllowsReadOnly(t *testing.T) {
	v := NewValidator(5000)
	allowed := []string{
		"SELECT * FROM Users",
		"select top 10 * from dbo.Users",
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
		"EXEC sp_who",
		"EXECUTE dbo.MyProc",
		"SELECT * INTO NewTable FROM Users",
		"SELECT * FROM Users; DROP TABLE Users",
		"USE master",
		"SET NOCOUNT ON",
		"DECLARE @x INT",
		"BEGIN TRAN",
		"MERGE INTO Users USING Src ON 1=1",
		"SELECT * FROM OPENROWSET('x','y','z')",
		"EXEC xp_cmdshell 'dir'",
		"SELECT * FROM Users WAITFOR DELAY '00:00:05'",
		"GRANT SELECT ON Users TO public",
		"BACKUP DATABASE db TO DISK = 'x'",
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
	// A DROP hidden by a block comment that is actually stripped: the DROP
	// becomes real once the comment is removed and must be rejected.
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
