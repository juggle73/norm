# norm - database "no ORM" tools

norm is tools to simplify working with postgresql database drivers (e.g. pgx) without ORM.

### Important changes in v2

Remove internal Model.currentObj, always using param object. 

Install:

```shell
go get -u github.com/juggle73/norm
```

Usage:

```go
package main

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/juggle73/norm"
	"os"
	"time"
)

type User struct {
	Id    int64  `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

func main() {
	pool, err := initDbClient()
	if err != nil {
		panic(err)
	}

	orm := norm.NewNorm(nil)

	user := User{}
	model := orm.M(&user)
	userId := 42

	sql := fmt.Sprintf("select %s from users where id=$1", model.Fields())

	err = pool.QueryRow(context.Background(), sql, userId).
		Scan(model.Pointers(&user)...)
	if err != nil {
		panic(err)
	}

	fmt.Println(user)
}

func initDbClient() (*pgxpool.Pool, error) {
	connString := fmt.Sprintf(
		"user=%s password=%s host=%s port=%s dbname=%s sslmode=%s pool_max_conns=100",
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_HOST"),
		os.Getenv("DB_PORT"),
		os.Getenv("DB_NAME"),
		os.Getenv("DB_SSL_MODE"))
	cfg, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()
	dbConnPool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}

	err = dbConnPool.Ping(context.Background())
	if err != nil {
		return nil, err
	}

	return dbConnPool, nil
}
```