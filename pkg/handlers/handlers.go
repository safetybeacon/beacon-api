package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rs/xid"

	"github.com/t4ke0/locations_api/db"
	"github.com/t4ke0/locations_api/pkg/api"
)

const AuthTokenName string = "X-Auth-Token"

type Handler struct {
	PostgresLink string
}

func (h Handler) ErrorHandler(c *gin.Context) {
	if r := recover(); r != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"message": fmt.Sprintf("%v", r),
		})
	}
}

func (h Handler) AuthorizeUser(c *gin.Context) (bool, error) {
	userID := c.Param("userid")
	token := c.GetHeader(AuthTokenName)

	id, err := strconv.Atoi(userID)
	if err != nil {
		return false, err
	}

	conn, err := db.NewDB(h.PostgresLink)
	if err != nil {
		return false, err
	}
	defer conn.Close()

	return conn.CheckUserToken(id, token)
}

// HandleRegister
func (h Handler) HandleRegister(c *gin.Context) {
	var req api.RegisterRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"message": fmt.Sprintf("Wrong JSON request %v", err),
		})
		return
	}

	if !req.Validate() {
		c.Status(http.StatusBadRequest)
		return
	}

	defer h.ErrorHandler(c)

	conn, err := db.NewDB(h.PostgresLink)
	if err != nil {
		panic(err)
	}

	defer func() {
		if err := conn.Close(); err != nil {
			panic(err)
		}
	}()

	if err := req.HashPassword(); err != nil {
		panic(err)
	}

	err = conn.NewUser(
		req.Email,
		req.Password,
		req.Firstname,
		req.Lastname,
		req.City,
		req.Country,
	)
	if err != nil && err == db.ErrUserEmailAlreadyExist {
		c.Status(http.StatusConflict)
		return
	}

	if err != nil {
		panic(err)
	}

	c.Status(http.StatusCreated)
}

// HandleLogin
func (h Handler) HandleLogin(c *gin.Context) {
	var req api.LoginRequest
	if err := c.BindJSON(&req); err != nil {
		c.Status(http.StatusBadRequest)
		return
	}

	if !req.Validate() {
		c.Status(http.StatusBadRequest)
		return
	}

	defer h.ErrorHandler(c)

	conn, err := db.NewDB(h.PostgresLink)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	user, err := conn.GetUser(req.Email)
	if err != nil {
		panic(err)
	}

	if user.Id == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"message": "user not found",
		})
		return
	}

	if !req.CompareHashAndPassword(user.Password) {
		c.Status(http.StatusUnauthorized)
		return
	}

	token := xid.New().String()
	tokenID, err := conn.NewToken(user.Id, token, req.Device)
	if err != nil {
		panic(err)
	}

	c.JSON(http.StatusOK, api.LoginResponse{
		Id:    tokenID,
		Token: token,
	})
}

// HandleLogout
func (h Handler) HandleLogout(c *gin.Context) {
	defer h.ErrorHandler(c)
	ok, err := h.AuthorizeUser(c)
	if err != nil {
		panic(err)
	}

	if !ok {
		c.Status(http.StatusUnauthorized)
		return
	}

	conn, err := db.NewDB(h.PostgresLink)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	userid, err := strconv.Atoi(c.Param("userid"))
	if err != nil {
		panic(err)
	}

	var req api.LogoutRequest
	if err := c.BindJSON(&req); err != nil {
		c.Status(http.StatusBadRequest)
		return
	}

	responseIDs := []int{}
	for _, tokenid := range req.Tokens {
		id, err := conn.DeleteToken(userid, tokenid)
		if err != nil {
			panic(err)
		}
		responseIDs = append(responseIDs, id)
	}

	c.JSON(http.StatusOK, api.LogoutResponse{
		Invalidated: responseIDs,
	})

}

func (h Handler) HandleAddLocation(c *gin.Context) {
	defer h.ErrorHandler(c)
	ok, err := h.AuthorizeUser(c)
	if err != nil {
		panic(err)
	}
	if !ok {
		c.Status(http.StatusUnauthorized)
		return
	}

	var req api.AddLocationRequest
	if err := c.BindJSON(&req); err != nil {
		c.Status(http.StatusBadRequest)
		return
	}

	if !req.Validate() {
		c.Status(http.StatusBadRequest)
		return
	}

	userid, err := strconv.Atoi(c.Param("userid"))
	if err != nil {
		panic(err)
	}

	token := c.GetHeader(AuthTokenName)

	conn, err := db.NewDB(h.PostgresLink)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	tokenid, err := conn.GetTokenID(userid, token)
	if err != nil {
		panic(err)
	}

	if tokenid == 0 {
		panic(fmt.Errorf("GetTokenID: failed to get tokenid"))
	}

	locationID, err := conn.NewLocation(
		userid, tokenid,
		req.Timestamp, req.Position.Latitude,
		req.Position.Longitude, req.Private,
	)
	if err != nil {
		panic(err)
	}

	c.JSON(http.StatusCreated, api.AddLocationResponse{
		Id: locationID,
	})
}

func (h Handler) HandleGetLocations(c *gin.Context) {
	defer h.ErrorHandler(c)
	conn, err := db.NewDB(h.PostgresLink)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	locations, err := conn.GetLocations()
	if err != nil {
		panic(err)
	}

	c.JSON(http.StatusOK, locations)
}

func (h Handler) AuthorizeHeader() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader(AuthTokenName)
		if strings.TrimSpace(token) == "" {
			c.Status(http.StatusUnauthorized)
			c.Abort()
		}

		defer h.ErrorHandler(c)

		conn, err := db.NewDB(h.PostgresLink)
		if err != nil {
			panic(err)
		}
		defer conn.Close()

		ok, err := conn.IsTokenExist(token)
		if err != nil {
			panic(err)
		}

		if !ok {
			c.Status(http.StatusUnauthorized)
			c.Abort()
		}

		c.Next()

	}
}
