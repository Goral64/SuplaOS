package migrate

import (
	"bytes"
	"strings"

	"github.com/Goral64/SuplaOS/internal/stack"
)

// StackDB is the database of the supla add-on, through its container.
type StackDB struct{ Stack *stack.Stack }

func (d StackDB) Query(q string) ([][]string, error) {
	var out bytes.Buffer
	err := d.Stack.Exec("supla-db", strings.NewReader(q), &out, "sh", "-c",
		`exec mariadb --batch --skip-column-names --default-character-set=utf8mb4 --user=supla --password="$MYSQL_PASSWORD" supla`)
	if err != nil {
		return nil, err
	}
	var rows [][]string
	for _, line := range strings.Split(strings.TrimRight(out.String(), "\n"), "\n") {
		if line != "" {
			rows = append(rows, strings.Split(line, "\t"))
		}
	}
	return rows, nil
}
