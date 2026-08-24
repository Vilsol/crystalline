package alpha

type Config struct{ Host string }

func Alpha() Config { return Config{Host: "a"} }
