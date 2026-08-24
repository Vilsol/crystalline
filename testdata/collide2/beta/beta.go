package beta

type Config struct{ Port int }

func Beta() Config { return Config{Port: 1} }
