package irodsuri

import (
	"fmt"
	"net"
	"net/url"
	"path"
	"strconv"
	"strings"

	irodstypes "github.com/cyverse/go-irodsclient/irods/types"
)

const Scheme = "irods"
const TicketQueryParam = "ticket"

type UserInfo struct {
	UserName string
	Zone     string
	Password string
}

type Parsed struct {
	URI      *url.URL
	UserInfo *UserInfo
	Host     string
	Port     int
	Path     string
	Ticket   string
}

func Parse(raw string) (*Parsed, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("empty iRODS URI")
	}

	uri, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parse iRODS URI: %w", err)
	}

	return ParseURL(uri)
}

func ParseURL(uri *url.URL) (*Parsed, error) {
	if uri == nil {
		return nil, fmt.Errorf("nil iRODS URI")
	}
	if !IsScheme(uri) {
		return nil, fmt.Errorf("invalid iRODS URI scheme %q", uri.Scheme)
	}

	userInfo, err := parseUserInfo(uri.User)
	if err != nil {
		return nil, err
	}

	port := 0
	if rawPort := strings.TrimSpace(uri.Port()); rawPort != "" {
		parsedPort, err := strconv.Atoi(rawPort)
		if err != nil {
			return nil, fmt.Errorf("parse iRODS URI port %q: %w", rawPort, err)
		}
		port = parsedPort
	}

	return &Parsed{
		URI:      uri,
		UserInfo: userInfo,
		Host:     strings.TrimSpace(uri.Hostname()),
		Port:     port,
		Path:     uri.Path,
		Ticket:   strings.TrimSpace(uri.Query().Get(TicketQueryParam)),
	}, nil
}

func IsScheme(uri *url.URL) bool {
	return uri != nil && strings.EqualFold(strings.TrimSpace(uri.Scheme), Scheme)
}

func Build(host string, port int, userInfo *UserInfo, absolutePath string) (*url.URL, error) {
	host = strings.TrimSpace(host)
	absolutePath = strings.TrimSpace(absolutePath)

	if host == "" {
		return nil, fmt.Errorf("empty host")
	}
	if port <= 0 {
		return nil, fmt.Errorf("invalid port %d", port)
	}
	if absolutePath == "" {
		return nil, fmt.Errorf("empty iRODS path")
	}
	if !strings.HasPrefix(absolutePath, "/") {
		return nil, fmt.Errorf("iRODS path must be absolute")
	}

	uri := &url.URL{
		Scheme: Scheme,
		Host:   net.JoinHostPort(host, fmt.Sprintf("%d", port)),
		Path:   path.Clean(absolutePath),
	}

	if userInfo != nil {
		userName := strings.TrimSpace(userInfo.UserName)
		if userName == "" {
			return nil, fmt.Errorf("empty iRODS user name")
		}

		if zone := strings.TrimSpace(userInfo.Zone); zone != "" {
			userName += "#" + zone
		}

		if userInfo.Password != "" {
			uri.User = url.UserPassword(userName, userInfo.Password)
		} else {
			uri.User = url.User(userName)
		}
	}

	return uri, nil
}

func BuildAnonymous(host string, port int, absolutePath string) (*url.URL, error) {
	return Build(host, port, nil, absolutePath)
}

func BuildWithTicket(host string, port int, userInfo *UserInfo, absolutePath string, ticket string) (*url.URL, error) {
	uri, err := Build(host, port, userInfo, absolutePath)
	if err != nil {
		return nil, err
	}

	ticket = strings.TrimSpace(ticket)
	if ticket == "" {
		return nil, fmt.Errorf("empty ticket")
	}

	query := uri.Query()
	query.Set(TicketQueryParam, ticket)
	uri.RawQuery = query.Encode()
	return uri, nil
}

func BuildForAccount(account *irodstypes.IRODSAccount, irodsPath string, includePassword bool) (*url.URL, error) {
	if account == nil {
		return nil, fmt.Errorf("nil iRODS account")
	}

	absolutePath := strings.TrimSpace(irodsPath)
	if absolutePath == "" {
		return nil, fmt.Errorf("empty iRODS path")
	}
	if !strings.HasPrefix(absolutePath, "/") {
		absolutePath = path.Join(account.GetHomeDirPath(), absolutePath)
	}

	userInfo := &UserInfo{
		UserName: account.ClientUser,
		Zone:     account.ClientZone,
	}
	if includePassword {
		userInfo.Password = account.Password
	}

	return Build(account.Host, account.Port, userInfo, absolutePath)
}

func BuildForAccountWithoutUserInfo(account *irodstypes.IRODSAccount, irodsPath string) (*url.URL, error) {
	if account == nil {
		return nil, fmt.Errorf("nil iRODS account")
	}

	absolutePath := strings.TrimSpace(irodsPath)
	if absolutePath == "" {
		return nil, fmt.Errorf("empty iRODS path")
	}
	if !strings.HasPrefix(absolutePath, "/") {
		absolutePath = path.Join(account.GetHomeDirPath(), absolutePath)
	}

	return Build(account.Host, account.Port, nil, absolutePath)
}

func BuildForTicketAccount(account *irodstypes.IRODSAccount, irodsPath string) (*url.URL, error) {
	if account == nil {
		return nil, fmt.Errorf("nil iRODS account")
	}
	if strings.TrimSpace(account.Ticket) == "" {
		return nil, fmt.Errorf("iRODS account does not include a ticket")
	}

	absolutePath := strings.TrimSpace(irodsPath)
	if absolutePath == "" {
		return nil, fmt.Errorf("empty iRODS path")
	}
	if !strings.HasPrefix(absolutePath, "/") {
		absolutePath = path.Join(account.GetHomeDirPath(), absolutePath)
	}

	userInfo := &UserInfo{
		UserName: account.ClientUser,
		Zone:     account.ClientZone,
	}

	return BuildWithTicket(account.Host, account.Port, userInfo, absolutePath, account.Ticket)
}

func AccountFromURL(uri *url.URL) (*irodstypes.IRODSAccount, string, error) {
	parsed, err := ParseURL(uri)
	if err != nil {
		return nil, "", err
	}
	if parsed.UserInfo == nil {
		return nil, "", fmt.Errorf("iRODS URI does not include user information")
	}

	userName := strings.TrimSpace(parsed.UserInfo.UserName)
	zone := strings.TrimSpace(parsed.UserInfo.Zone)
	password := parsed.UserInfo.Password
	if userName == "" || zone == "" || password == "" {
		return nil, "", fmt.Errorf("iRODS URI must include username, zone, and password to build an account")
	}
	if parsed.Host == "" || parsed.Port <= 0 {
		return nil, "", fmt.Errorf("iRODS URI must include host and port to build an account")
	}

	account, err := irodstypes.CreateIRODSAccount(
		parsed.Host,
		parsed.Port,
		userName,
		zone,
		irodstypes.AuthSchemeNative,
		password,
		"",
	)
	if err != nil {
		return nil, "", fmt.Errorf("create iRODS account from URI: %w", err)
	}

	return account, parsed.Path, nil
}

func TicketAccountFromURL(uri *url.URL) (*irodstypes.IRODSAccount, string, error) {
	parsed, err := ParseURL(uri)
	if err != nil {
		return nil, "", err
	}
	if parsed.UserInfo == nil {
		return nil, "", fmt.Errorf("iRODS URI does not include user information")
	}

	userName := strings.TrimSpace(parsed.UserInfo.UserName)
	zone := strings.TrimSpace(parsed.UserInfo.Zone)
	ticket := strings.TrimSpace(parsed.Ticket)
	if userName == "" || zone == "" || ticket == "" {
		return nil, "", fmt.Errorf("iRODS URI must include username, zone, and ticket to build a ticket account")
	}
	if parsed.Host == "" || parsed.Port <= 0 {
		return nil, "", fmt.Errorf("iRODS URI must include host and port to build a ticket account")
	}

	account, err := irodstypes.CreateIRODSAccountForTicket(
		parsed.Host,
		parsed.Port,
		userName,
		zone,
		irodstypes.AuthSchemeNative,
		"",
		ticket,
		"",
	)
	if err != nil {
		return nil, "", fmt.Errorf("create iRODS ticket account from URI: %w", err)
	}

	return account, parsed.Path, nil
}

func parseUserInfo(user *url.Userinfo) (*UserInfo, error) {
	if user == nil {
		return nil, nil
	}

	rawUserName := strings.TrimSpace(user.Username())
	if rawUserName == "" {
		return nil, fmt.Errorf("empty iRODS user name in URI")
	}

	userName := rawUserName
	zone := ""
	if before, after, ok := strings.Cut(rawUserName, "#"); ok {
		userName = strings.TrimSpace(before)
		zone = strings.TrimSpace(after)
	}

	password, _ := user.Password()

	return &UserInfo{
		UserName: userName,
		Zone:     zone,
		Password: password,
	}, nil
}
