package common

import "github.com/sirupsen/logrus"

const Version = "0.1.0-dev"

func InitLogger() {
	logrus.SetFormatter(&logrus.JSONFormatter{})
}