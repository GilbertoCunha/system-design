package internal

type PgUrlRepo struct {
	host     string
	port     int
	dbName   string
	user     string
	password string
}

func NewPgUrlRepo(c *AppConfig) *PgUrlRepo {
	return &PgUrlRepo{
		host:     c.Postgres.Host,
		port:     c.Postgres.Port,
		dbName:   c.Postgres.DbName,
		user:     c.Postgres.User,
		password: c.Postgres.Password,
	}
}

func (r *PgUrlRepo) GetLongUrl(shortUrl string) (string, error) {
	return "", nil
}

func (r *PgUrlRepo) PutShortUrl(longUrl string) (string, error) {
	return "", nil
}
