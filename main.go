package main

import (
	"bytes"
	"github.com/astaxie/beego/config"
	"github.com/dchest/captcha"
	"github.com/garyburd/redigo/redis"
	"github.com/gorilla/mux"
	"io"
	"log"
	"net/http"
	"time"
)

var initConf config.Configer

func newCaptchaHandle(w http.ResponseWriter, r *http.Request) {
	io.WriteString(w, captcha.New())
}

func checkHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; chaset=utf-8")
	r.ParseForm()
	if len(r.Form["captchaId"]) <= 0 {
		io.WriteString(w, "0")
		return
	}
	if len(r.Form["captchaSolution"]) <= 0 {
		io.WriteString(w, "0")
		return
	}
	if !captcha.VerifyString(r.Form["captchaId"][0], r.Form["captchaSolution"][0]) {
		io.WriteString(w, captcha.New())
	} else {
		io.WriteString(w, "1")
	}
}

func imageHandle(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	captchaId := vars["id"]
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.Header().Set("Content-Type", "image/png")
	var content bytes.Buffer
	captcha.WriteImage(&content, captchaId, 246, 80)
	http.ServeContent(w, r, captchaId, time.Time{}, bytes.NewReader(content.Bytes()))
}

type RedisStore struct {
	poollist *redis.Pool
}

func (s *RedisStore) Set(id string, digits []byte) {
	conn := s.poollist.Get()
	defer conn.Close()
	expire, err := initConf.Int("keyExpire")
	if err != nil {
		expire = 600
	}
	conn.Do("SET", initConf.String("redisPrefix")+id, digits, "EX", expire)
}

func (s *RedisStore) Get(id string, clear bool) (digits []byte) {
	conn := s.poollist.Get()
	defer conn.Close()
	redisValue, err := conn.Do("GET", initConf.String("redisPrefix")+id)
	if clear {
		conn.Do("DEL", initConf.String("redisPrefix")+id)
	}
	if err != nil {
		return []byte("")
	}
	return redisValue.([]byte)
}

func main() {
	var err error
	initConf, err = config.NewConfig("ini", "./conf/app.conf")
	if err != nil {
		initConf = config.NewFakeConfig()
		initConf.Set("redisAddress", "localhost:6379")
		initConf.Set("httpport", "8001")
	}
	redisStore := new(RedisStore)
	redisStore.poollist = &redis.Pool{
		Dial: func() (redis.Conn, error) {
			conn, err := redis.Dial("tcp", initConf.String("redisAddress"))
			if err != nil {
				log.Fatal(err)
				panic(err)
			}
			return conn, err
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			if time.Since(t) < time.Minute {
				return nil
			}
			_, err := c.Do("PING")
			log.Fatal(err)
			return err
		},
		MaxIdle:     5,
		IdleTimeout: 60 * time.Second,
	}
	captcha.SetCustomStore(redisStore)
	captcha.New() // 测试一下输出，保证没有出问题

	r := mux.NewRouter()
	r.HandleFunc("/new", newCaptchaHandle)
	r.HandleFunc("/check", checkHandler)
	r.HandleFunc("/image/{id}", imageHandle)
	http.Handle("/", r)
	log.Println("captcha service on port: " + initConf.String("httpport"))
	if err := http.ListenAndServe("localhost:"+initConf.String("httpport"), nil); err != nil {
		log.Fatal(err)
	}
}
