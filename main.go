package main

import (
	"encoding/json"
	"crypto/md5"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type LinkRequest struct {
	Address  string `form:"address" json:"address" xml:"address" binding:"required"`
	Password string `form:"password" json:"password" xml:"password" binding:"required"`
}

type Link struct {
	gorm.Model
	Id     uint16
	Target string
}

type Config struct {
	Prefix   string `json:"prefix"`
	db       *gorm.DB
	DBname   string `json:"db"`
	Password string `json:"password"`
}

func main() {
	file, err := ioutil.ReadFile("config.json")

	if err != nil {
		log.Fatalln("config.json not found")
	}

	var config Config

	json.Unmarshal(file, &config) // Initialize the configuration from the config.json file

	println("URL shortener address prefix: /" + config.Prefix)

	if config.Password == "" {
		log.Fatalln("no password specified in config.json")
	}

	println("password: " + config.Password[0:1] + "*********")

	if config.DBname == "" {
		log.Fatalln("no database file specified in config.json")
	}

	println("using database " + config.DBname)

	db, err := gorm.Open(sqlite.Open(config.DBname), &gorm.Config{})

	if err != nil {
		log.Fatalln("failed to connect database")
	}
	config.db = db
	db.AutoMigrate(&Link{})

	rand.Seed(time.Now().UnixNano())

	gin.SetMode(gin.ReleaseMode)

	router := gin.Default()
	router.SetTrustedProxies(nil) // If you're using a CDN or some other kind of proxy this does not work.

	//
	// Loads all HTML templates within the templates folder
	//
	router.LoadHTMLGlob("templates/*") //LoadHTMLGlob enables loading a batch of templates at once

	///
	/// API endpoint for adding the links
	///
	router.POST("/"+config.Prefix+"add", func(context *gin.Context) {
		requestBody := LinkRequest{}
		context.Bind(&requestBody)

		if md5.Sum([]byte(requestBody.Password)) != md5.Sum([]byte(config.Password)) { // Exits if a wrong password was provided
			context.JSON(http.StatusUnauthorized, gin.H{})
			return
		}

		link := Link{Target: "some random text"}
		var rng uint16

		for link.Target != "" {
			rng = uint16(rand.Uint32())
			link.Target = ""               // Set target to an empty string...
			db.First(&link, "id = ?", rng) // ...except if the id happens to already be in use
		}

		link.Id = rng
		link.Target = requestBody.Address

		db.Create(&link)
		db.Save(&link)

		context.JSON(200, gin.H{
			"address": "/" + config.Prefix + parseIdInt(rng),
		})
	})

	///
	/// Redirect
	///
	router.GET("/"+config.Prefix+":id", func(context *gin.Context) {
		var link Link
		i, err := parseIdString(context.Param("id"))

		if err != nil {
			context.HTML(http.StatusBadRequest, "badrequest.html", gin.H{"title": "400 - Bad Request"})
			return
		}

		db.First(&link, "id = ?", i)

		if link.Target == "" {
			context.HTML(http.StatusBadRequest, "badrequest.html", gin.H{"title": "400 - Bad Request"})
			return
		}

		context.Redirect(http.StatusMovedPermanently, link.Target)
	})

	///
	/// /prefix/
	///
	router.GET("/"+config.Prefix, func(context *gin.Context) {
		context.HTML(http.StatusBadRequest, "badrequest.html", gin.H{"title": "400 - Bad Request"})
	})

	///
	///	static route for all resources within the "resources" folder
	///
	router.Static("/resources", "./resources")

	router.Run(":8080")
}

func parseIdString(idString string) (uint16, error) {
	i, err := strconv.ParseUint(idString, 16, 32)
	return uint16(i), err
}

func parseIdInt(idInt uint16) string {
	return strconv.FormatUint(uint64(idInt), 16)
}
