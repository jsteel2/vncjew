package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/evangwt/go-vncproxy"
	"github.com/gin-gonic/gin"
	"golang.org/x/net/websocket"
)

func errorMSG(c *gin.Context, err error) {
	c.String(http.StatusInternalServerError, "%s", err.Error())
}

func main() {
	r := gin.Default()
	r.LoadHTMLGlob("templates/*")
	r.StaticFile("/favicon.ico", "./res/favicon.ico")
	os.Mkdir("./screenshots", os.ModePerm)
	r.Static("/screenshots", "./screenshots")
	r.Static("/novnc", "./novnc")

	db, err := NewDB()
	if err != nil {
		log.Fatal(err)
	}

	wsServ := NewWSServ(db)

	admin := gin.BasicAuth(CFGAdminAccount)
	client := gin.BasicAuth(CFGClientAccount)

	vncProxy := vncproxy.New(&vncproxy.Config{
		TokenHandler: func(r *http.Request) (string, error) {
			return r.URL.Query().Get("token"), nil
		},
	})

	r.GET("/", func(c *gin.Context) {
		hosts, err := db.Count(&Host{})
		if err != nil {
			errorMSG(c, err)
			return
		}
		services, err := db.Count(&Service{})
		if err != nil {
			errorMSG(c, err)
			return
		}
		c.HTML(http.StatusOK, "index.html", gin.H{
			"hosts": hosts,
			"services": services,
			"vncs": services,
		})
	})

	r.GET("/search", func(c *gin.Context) {
		offset, _ := strconv.Atoi(c.Query("offset"))
		if offset < 0 {
			offset = 0
		}
		amt, _ := strconv.Atoi(c.Query("amt"))
		if amt <= 0 {
			amt = 25
		}

		services, err := db.Search(c.Query("query"), offset, amt)
		if err != nil {
			errorMSG(c, err)
			return
		}

		c.HTML(http.StatusOK, "search.html", gin.H{
			"results": services,
			"query": c.Query("query"),
			"next": offset + amt,
			"amt": amt,
		})
	})

	r.GET("/host/:ip", func(c *gin.Context) {
		host, err := db.GetHost(c.Param("ip"))
		if err != nil {
			errorMSG(c, err)
			return
		}

		c.HTML(http.StatusOK, "host.html", gin.H{
			"host": host,
		})
	})

	r.GET("/admin/status", admin, func(c *gin.Context) {
		c.HTML(http.StatusOK, "admin_status.html", gin.H{
			"clients": wsServ.SendStatus(),
		})
	})

	r.GET("/admin/start", admin, func(c *gin.Context) {
		c.String(http.StatusOK, "%s", wsServ.SendStart())
	})

	r.GET("/admin/stop", admin, func(c *gin.Context) {
		c.String(http.StatusOK, "%s", wsServ.SendStop())
	})

	r.POST("/admin/deleteHost", admin, func(c *gin.Context) {
		err := db.DeleteHost(c.PostForm("ip"))
		if err != nil {
			errorMSG(c, err)
			return
		}
		c.String(http.StatusOK, "Successfully deleted host")
	})

	r.POST("/admin/deleteService", admin, func(c *gin.Context) {
		err := db.DeleteService(c.PostForm("ip"), c.PostForm("port"))
		if err != nil {
			errorMSG(c, err)
			return
		}
		c.String(http.StatusOK, "Successfully deleted service")
	})

	r.POST("/admin/refresh", admin, func(c *gin.Context) {
		err := AddVNC(c.PostForm("ip"), c.PostForm("port"),
			c.PostForm("username"), c.PostForm("password"), db)
		if err != nil {
			errorMSG(c, err)
		} else {
			c.String(http.StatusOK, "Refreshed VNC successfully")
		}
	})

	r.GET("/api/database", func(c *gin.Context) {
		hosts, err := db.GetHosts()
		if err != nil {
			errorMSG(c, err)
			return
		}
		m, err := json.Marshal(hosts)
		if err != nil {
			errorMSG(c, err)
			return
		}
		c.String(http.StatusOK, "%s", m)
	})

	r.GET("/api/client", client, func(c *gin.Context) {
		websocket.Handler(wsServ.ServeWS).ServeHTTP(c.Writer, c.Request)
	})

	r.GET("/websockify", func(c *gin.Context) {
		websocket.Handler(vncProxy.ServeWS).ServeHTTP(c.Writer, c.Request)
	})

	log.Fatal(r.Run())
}
