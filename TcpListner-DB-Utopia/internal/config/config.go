package config

import (
	"github.com/spf13/viper"
)

type Config struct {
	Env        string
	DBHost     string
	DBName     string
	DBUser     string
	DBPassword string
	TcpPort    string
	// TcpPort       []int
	TcpHost       string
	DjangoHost    string
	LogPath       string
	RedisHost     string
	RedisPort     string
	RedisPassword string
	RedisDB       int
}

func New() *Config {
	viper.AutomaticEnv()

	// portStrings := strings.Split(viper.GetString("TCP_PORT"), ",")
	// var ports []int
	// for _, ps := range portStrings {
	// 	if p, err := strconv.Atoi(strings.TrimSpace(ps)); err == nil {
	// 		ports = append(ports, p)
	// 	}
	// }

	return &Config{
		Env:           viper.GetString("ENV"),
		DBHost:        viper.GetString("DB_HOST"),
		DBName:        viper.GetString("DB_NAME"),
		DBUser:        viper.GetString("DB_USER"),
		DBPassword:    viper.GetString("DB_PASSWORD"),
		TcpPort:       viper.GetString("TCP_PORT"),
		TcpHost:       viper.GetString("TCP_HOST"),
		DjangoHost:    viper.GetString("DJANGO_HOST"),
		LogPath:       viper.GetString("LOG_PATH"),
		RedisHost:     viper.GetString("REDIS_HOST"),
		RedisPort:     viper.GetString("REDIS_PORT"),
		RedisPassword: viper.GetString("REDIS_PASSWORD"),
		RedisDB:       viper.GetInt("REDIS_DB"),
	}
}
