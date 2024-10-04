package examples

import (
	"context"
	"fmt"
	"github.com/juggle73/norm"
)

type User struct {
	Id    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

func QueryRow() {
	pool, err := initDbClient()
	if err != nil {
		panic(err)
	}

	orm := norm.NewNorm(nil)

	user := User{}
	model := orm.M(&user)
	userId := 42

	sql := fmt.Sprintf("select %s from users where id=$1", model.DbNamesCsv("", ""))

	err = pool.QueryRow(context.Background(), sql, userId).
		Scan(model.Pointers("")...)
	if err != nil {
		panic(err)
	}

	fmt.Println(user)
}
