package app

import (
	"os"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"github.com/gorilla/mux"
	"gopkg.in/mgo.v2"
	"strings"
	l "github.com/eriklindqvist/recepies/app/lib"
	c "github.com/eriklindqvist/recepies/app/controllers"
	jwt "github.com/dgrijalva/jwt-go"
)

type Route struct {
	Name        string
	Method      string
	Pattern     string
	HandlerFunc http.HandlerFunc
}

type Routes []Route
type Endpoint func(w http.ResponseWriter, r *http.Request) ([]byte, error)

type Scope struct {
  Entity string `json:"ent"`
  Actions []string `json:"act"`
}

type User struct {
    Username string `json:"usr"`
    Scopes []Scope `json:"scp"`
    jwt.StandardClaims
}

func getSession() *mgo.Session {
		host := "mongodb://" + l.Getenv("MONGODB_HOST", "localhost")
    s, err := mgo.Dial(host)
		log.Printf("host: %s", host)
    // Check if connection error, is mongo running?
    if err != nil {

        panic(err)
    }
    return s
}

var rc = c.NewRecipeController(getSession())
var routes = NewRoutes(*rc)

func NewRouter() *mux.Router {
	router := mux.NewRouter().StrictSlash(true)
	for _, route := range routes {
		var handler http.Handler
		handler = route.HandlerFunc
		handler = Logger(handler, route.Name)

		router.
			Methods(route.Method).
			Path(route.Pattern).
			Name(route.Name).
			Handler(handler)
	}

	return router
}

func validates(scopes []Scope, entity string, action string) bool {
	for _, scope:= range scopes {
		if (scope.Entity == entity) {
			for _, act := range scope.Actions {
				if act == action {
					return true
				}
			}
			return false
		}
	}
	return false
}

func contains(arr []string, str string) bool {
  for _, a := range arr {
      if a == str {
          return true
      }
  }
  return false
}

func Handle(entity string, action string, endpoint Endpoint, w http.ResponseWriter, r *http.Request) {
	protected := strings.Split(os.Getenv("PROTECTED_ENDPOINTS"),",")

	if contains(protected, entity+":"+action) {
		authorization := r.Header.Get("Authorization")
		regex, _ := regexp.Compile("(?:Bearer *)([^ ]+)(?: *)")
		matches := regex.FindStringSubmatch(authorization)

		if len(matches) != 2 {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
		 	return
		}

		jwtToken := matches[1]
		secret := []byte(os.Getenv("SECRET"))

		// parse token
		token, err := jwt.ParseWithClaims(jwtToken, &User{}, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("Unknown signing method")
			}

			return secret, nil
		})

		if err != nil {
			http.Error(w, "Unauthorized: " + err.Error(), http.StatusUnauthorized)
			return
		}

		// extract claims
		user, ok := token.Claims.(*User)

		if !ok || !token.Valid {
			http.Error(w, "Unauthorized: " + err.Error(), http.StatusUnauthorized)
			return
		}

		log.Printf("scopes: %s", user.Scopes)

		if (!validates(user.Scopes, entity, action)) {
			http.Error(w, "Unauthorized: Insufficient privileges", http.StatusUnauthorized)
			return
		}
	}

	setContentType(w)

	body, err := endpoint(w, r)

	if err != nil {
		switch e := err.(type) {
		case l.Error:
			http.Error(w, e.Error(), e.Status())
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}

		return
	}

	writeBody(w, body)
}

func setContentType(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
}

func writeNotFound(w http.ResponseWriter) {
	w.WriteHeader(http.StatusNotFound)
}

func writeBody(w http.ResponseWriter, body []byte) {
	fmt.Fprintf(w, "%s", body)
}
