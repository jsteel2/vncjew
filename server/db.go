package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/alecthomas/participle/v2/lexer"
	"github.com/glebarez/sqlite"
	"github.com/ipinfo/go/v2/ipinfo"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type DB struct {
	db *gorm.DB
}

type Host struct {
	Ip string `gorm:"primaryKey"`
	Asn string
	Org string
	Country string
	City string
	Region string
	Hostname string
	Services []Service `gorm:"foreignKey:HostIp"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Service struct {
	HostIp string `gorm:"primaryKey"`
	Port uint16 `gorm:"primaryKey"`
	Username string
	Password string
	ClientName string
	Text string
	CreatedAt time.Time
	UpdatedAt time.Time
	Width int
	Height int
	Type string
}

var ipClient = ipinfo.NewClient(nil, nil, CFGIPInfoToken)

func NewDB() (*DB, error) {
	db, err := gorm.Open(sqlite.Open(CFGDb), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	db.AutoMigrate(&Host{}, &Service{})
	sql, err := db.DB()
	if err != nil {
		return nil, err
	}
	sql.SetMaxOpenConns(1) // so that sqlite never locks...?
	return &DB{db}, nil
}

var sqlLexer = lexer.MustSimple([]lexer.SimpleRule{
	{"SQL", `\d+|host\.ip|host\.asn|host\.org|host\.country|host\.city` +
		`|host\.region|host\.hostname|host\.created_at|host\.updated_at` +
		`|username|password|text|client_name|port|services\.created_at` +
		`|services\.updated_at|width|height` +
		`|BETWEEN|ORDER|NULL|LIKE|ASC|DESC|NOT|AND|OR|BY|IS|RANDOM\(\)|>=|<=|=|<>|<|>`},
	{"String", `"(\\"|[^"])*"`},
	{"whitespace", `[ \t\r\n]+`},
})
var tokSQL = sqlLexer.Symbols()["SQL"]
var tokString = sqlLexer.Symbols()["String"]

func (db *DB) Search(query string, offset, amount int) ([]Service, error) {
	lex, err := sqlLexer.Lex("", strings.NewReader(query))
	if err != nil {
		return nil, err
	}

	var sql strings.Builder
	var sqlArgs []interface{}

	for {
		token, err := lex.Next()
		if err != nil {
			return nil, err
		}
		if token.EOF() {
			break
		}

		if token.Type == tokSQL {
			sql.WriteString(token.Value)
			sql.WriteRune(' ')
		} else if token.Type == tokString {
			var val string
			if token.Value[0] == '"' {
				val = token.Value[1:][:len(token.Value) - 2]
			} else {
				val = token.Value
			}
			val = strings.ReplaceAll(val, `\`, "")
			sql.WriteString("? ")
			sqlArgs = append(sqlArgs, val)
		}
	}

	q1 := db.db.Offset(offset).Limit(amount)
	q := q1.Joins("JOIN hosts host ON host.ip = host_ip").Where(sql.String(), sqlArgs...)
	var services []Service

	err = q.Find(&services).Error
	return services, err
}

func (db *DB) AddService(ip, port, ocr string, info *VNCInfo) error {
	ipInfo, err := ipClient.GetIPInfo(net.ParseIP(ip))
	if err != nil {
		return err
	}

	_port, err := strconv.Atoi(port)
	if err != nil {
		return err
	}
	if _port > 0xffff {
		return errors.New("invalid port")
	}
	uport := uint16(_port)

	splitOrg := strings.Split(ipInfo.Org, " ")

	err = db.db.Clauses(clause.OnConflict{
		UpdateAll: true,
	}).Create(Host{
		Ip: ip,
		Asn: splitOrg[0],
		Org: strings.Join(splitOrg[1:], " "),
		Country: ipInfo.Country,
		City: ipInfo.City,
		Region: ipInfo.Region,
		Hostname: ipInfo.Hostname,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}).Error

	if err != nil {
		return err
	}

	err = db.db.Clauses(clause.OnConflict{
		UpdateAll: true,
	}).Create(Service{
		HostIp: ip,
		Port: uport,
		Username: info.Username,
		Password: info.Password,
		ClientName: info.ClientName,
		Text: ocr,
		Width: info.Width,
		Height: info.Height,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Type: "VNC",
	}).Error

	return err
}

func (db *DB) GetHost(ip string) (Host, error) {
	var host Host
	err := db.db.Preload("Services", func(db *gorm.DB) *gorm.DB {
		return db.Order("port")
	}).First(&host, "ip = ?", ip).Error

	return host, err
}

func (db *DB) DeleteHost(ip string) error {
	var services []Service
	err := db.db.Where("host_ip = ?", ip).Find(&services).Error
	if err != nil {
		return err
	}
	for _, s := range services {
		err = db.DeleteService(ip, strconv.Itoa(int(s.Port)))
		if err != nil {
			return err
		}
	}
	return nil
}

func (db *DB) DeleteService(ip string, port string) error {
	err := db.db.Unscoped().Where("host_ip = ? AND port = ?", ip, port).Delete(&Service{}).Error
	if err != nil {
		return err
	}
	var n int64
	err = db.db.Model(&Service{}).Where("host_ip = ?", ip).Count(&n).Error
	if err != nil {
		return err
	}
	if n == 0 {
		err = db.db.Unscoped().Where("ip = ?", ip).Delete(&Host{}).Error
		if err != nil {
			return err
		}
	}
	return os.Remove(fmt.Sprintf("./screenshots/%s_%s.jpeg", ip, port))
}

func (db *DB) GetHosts() ([]Host, error) {
	var hosts []Host
	err := db.db.Preload("Services").Find(&hosts).Error

	return hosts, err
}

func (db *DB) Count(v interface{}) (int64, error) {
	var n int64
	return n, db.db.Model(v).Count(&n).Error
}
