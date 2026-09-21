package postgres

import (
	"reflect"
	"testing"
)

func TestCleanDefinition(t *testing.T) {
	def := `-- line comment
CREATE OR REPLACE FUNCTION test() RETURNS void AS $$
/* block comment */
SELECT 'string literal';
$$ LANGUAGE plpgsql;`

	clean := cleanDefinition(def)
	if clean == "" {
		t.Errorf("expected non-empty clean definition")
	}
	if contains(clean, "line comment") || contains(clean, "block comment") {
		t.Errorf("comments should be stripped, got: %s", clean)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestNormalizeRef(t *testing.T) {
	tests := []struct {
		input    string
		wantBare string
		wantFull string
	}{
		{"\"my_table\"", "my_table", "my_table"},
		{"public.users", "users", "public.users"},
		{"\"public\".\"users\"", "users", "public.users"},
		{"db.public.users", "users", "public.users"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			bare, full := normalizeRef(tt.input)
			if bare != tt.wantBare || full != tt.wantFull {
				t.Errorf("normalizeRef(%q) = (%q, %q), want (%q, %q)", tt.input, bare, full, tt.wantBare, tt.wantFull)
			}
		})
	}
}

func TestExtractTransactionUsage(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "contains begin and commit",
			input: "BEGIN; INSERT INTO tbl VALUES (1); COMMIT;",
			want:  []string{"BEGIN/START TRANSACTION", "COMMIT"},
		},
		{
			name:  "contains rollback and savepoint",
			input: "SAVEPOINT sp1; ROLLBACK TO sp1;",
			want:  []string{"ROLLBACK", "SAVEPOINT"},
		},
		{
			name:  "none",
			input: "SELECT 1;",
			want:  []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractTransactionUsage(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("extractTransactionUsage() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestExtractDynamicSQL(t *testing.T) {
	sql := `EXECUTE format('SELECT * FROM %I', table_name);`
	got := extractDynamicSQL(sql)
	if len(got) == 0 {
		t.Errorf("expected dynamic sql to be detected")
	}
}

func TestExtractCursors(t *testing.T) {
	sql := `DECLARE cur1 CURSOR FOR SELECT * FROM users; OPEN cur2;`
	got := extractCursors(sql)
	if len(got) != 2 {
		t.Errorf("expected 2 cursors, got: %v", got)
	}
}

func TestExtractTempTables(t *testing.T) {
	sql := `CREATE TEMP TABLE temp_users AS SELECT * FROM users;`
	got := extractTempTables(sql)
	if len(got) == 0 {
		t.Errorf("expected temp table to be detected, got: %v", got)
	}
}

func TestExtractErrorHandling(t *testing.T) {
	sql := `BEGIN EXCEPTION WHEN OTHERS THEN RAISE EXCEPTION 'err'; GET DIAGNOSTICS x = ROW_COUNT; END;`
	got := extractErrorHandling(sql)
	if len(got) < 3 {
		t.Errorf("expected at least 3 error handling features, got: %v", got)
	}
}

func TestExtractValidationAndBusinessRules(t *testing.T) {
	def := `-- Business Rule: Only active users can place orders
IF user_status != 'active' THEN
    RAISE EXCEPTION 'User not active';
END IF;`

	biz := extractBusinessRules(def)
	if len(biz) == 0 {
		t.Errorf("expected business rule to be extracted")
	}

	val := extractValidationRules(def)
	if len(val) == 0 {
		t.Errorf("expected validation rule to be extracted")
	}
}

func TestCycleDetection(t *testing.T) {
	stack := []string{"table_a", "table_b", "table_c"}
	cycle := captureCycle(stack, "table_b")
	expected := []string{"table_b", "table_c", "table_b"}
	if !reflect.DeepEqual(cycle, expected) {
		t.Errorf("captureCycle() = %v, want %v", cycle, expected)
	}

	cycles := [][]string{
		{"a", "b", "a"},
		{"a", "b", "a"},
		{"b", "c", "b"},
	}
	deduped := dedupeCycles(cycles)
	if len(deduped) != 2 {
		t.Errorf("expected 2 unique cycles, got: %v", deduped)
	}
}

func TestUniqueSortedBool(t *testing.T) {
	set := map[string]bool{"zeta": true, "alpha": true, "beta": true}
	sorted := uniqueSortedBool(set)
	expected := []string{"alpha", "beta", "zeta"}
	if !reflect.DeepEqual(sorted, expected) {
		t.Errorf("uniqueSortedBool() = %v, want %v", sorted, expected)
	}
}
