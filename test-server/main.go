package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

const (
	// DefaultPort is the default port for the server
	DefaultHost = "0.0.0.0:8080"
)

func main() {
	r := gin.Default()
	r.GET("/", handleHello)
	r.GET("/headers", handleHeaders)
	r.Run(DefaultHost)
}

func handleHello(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"message": "Slothperator test server is up and running!",
	})
}

func handleHeaders(c *gin.Context) {
	// this function is used to test the headers of the request
	// it will return the headers of the request
	c.JSON(http.StatusOK, c.Request.Header)
}
