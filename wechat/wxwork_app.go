package wechat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// WxWorkAppGroupMessageAPI is the api to get the app access token
const WxWorkAppTokenAPI = "https://qyapi.weixin.qq.com/cgi-bin/gettoken"

// WxWorkAppUploadMediaAPI is the api to upload the wxwork app media
const WxWorkAppUploadMediaAPI = "https://qyapi.weixin.qq.com/cgi-bin/media/upload"

// WxWorkAppUploadImageAPI is the api to upload the wxwork app image
const WxWorkAppUploadImageAPI = "https://qyapi.weixin.qq.com/cgi-bin/media/uploadimg"

// WxWorkAppMessageAPI is the api to send messages to wxwork user/party/tag
const WxWorkAppMessageAPI = "https://qyapi.weixin.qq.com/cgi-bin/message/send"

// WxWorkAppGroupMessageAPI is the api to send messages to wxwork group
const WxWorkAppGroupMessageAPI = "https://qyapi.weixin.qq.com/cgi-bin/appchat/send"

// WxWorkAppCreateGroupAPI is the api to create the wxwork group
const WxWorkAppCreateGroupAPI = "https://qyapi.weixin.qq.com/cgi-bin/appchat/create"

// WxWorkAppUpdateGroupAPI is the api to update the wxwork group
const WxWorkAppUpdateGroupAPI = "https://qyapi.weixin.qq.com/cgi-bin/appchat/update"

// WxWorkAppGetGroupAPI is the api to get the wxwork group
const WxWorkAppGetGroupAPI = "https://qyapi.weixin.qq.com/cgi-bin/appchat/get"

// WxWorkAppTimeout is the wxwork app default timeout
const WxWorkAppTimeout = time.Second * 10

// WxWorkAppStatusOK is the ok status of api call
const WxWorkAppStatusOK = 0

// See doc https://work.weixin.qq.com/api/doc/90000/90139/90313
const WxWorkCodeAccessTokenExpired = 42001

const (
	WxWorkAppMessageTypeText     = "text"
	WxWorkAppMessageTypeImage    = "image"
	WxWorkAppMessageTypeVoice    = "voice"
	WxWorkAppMessageTypeVideo    = "video"
	WxWorkAppMessageTypeFile     = "file"
	WxWorkAppMessageTypeTextCard = "textcard"
	WxWorkAppMessageTypeNews     = "news"
	WxWorkAppMessageTypeMpNews   = "mpnews"
	WxWorkAppMessageTypeMarkdown = "markdown"
	// The following two message type are only supported by simple message
	WxWorkAppMessageTypeMiniProgramNotice = "miniprogram_notice"
	WxWorkAppMessageTypeTaskCard          = "taskcard"
)

const (
	WxWorkAppMediaTypeImage = "image"
	WxWorkAppMediaTypeVoice = "voice"
	WxWorkAppMediaTypeVideo = "video"
	WxWorkAppMediaTypeFile  = "file"
)

type WxWorkAppTokenResp struct {
	ErrCode     int    `json:"errcode"`
	ErrMessage  string `json:"errmsg"`
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

type WxWorkAppMessageResp struct {
	ErrCode      int    `json:"errcode"`
	ErrMessage   string `json:"errmsg"`
	InvalidUser  string `json:"invaliduser"`
	InvalidParty string `json:"invalidparty"`
	InvalidTag   string `json:"invalidtag"`
}

type WxWorkAppGroupMessageResp struct {
	ErrCode    int    `json:"errcode"`
	ErrMessage string `json:"errmsg"`
}

type WxWorkAppUploadMediaResp struct {
	ErrCode    int    `json:"errcode"`
	ErrMessage string `json:"errmsg"`
	Type       string `json:"type"`
	MediaID    string `json:"media_id"`
	CreatedAt  string `json:"created_at"`
}

type WxWorkAppUploadImageResp struct {
	ErrCode    int    `json:"errcode"`
	ErrMessage string `json:"errmsg"`
	URL        string `json:"url"`
}

type WxWorkAppCreateGroupOptions struct {
	ChatID string
}

type WxWorkAppUpdateGroupOptions struct {
	Name        string
	Owner       string
	AddUserList []string
	DelUserList []string
}

type WxWorkAppCreateGroupResp struct {
	ErrCode    int    `json:"errcode"`
	ErrMessage string `json:"errmsg"`
	ChatID     string `json:"chatid"`
}

type WxWorkAppUpdateGroupResp struct {
	ErrCode    int    `json:"errcode"`
	ErrMessage string `json:"errmsg"`
}

type WxWorkAppGetGroupResp struct {
	ErrCode    int            `json:"errcode"`
	ErrMessage string         `json:"errmsg"`
	ChatInfo   WxWorkAppGroup `json:"chat_info"`
}

type WxWorkAppGroup struct {
	ChatID   string   `json:"chatid"`
	Name     string   `json:"name"`
	Owner    string   `json:"owner"`
	UserList []string `json:"userlist"`
}

type WxWorkAppMessageSendOptions struct {
	Safe                   bool
	EnableIDTrans          bool
	EnableDuplicateCheck   bool
	DuplicateCheckInterval int
}

type WxWorkAppNewsMessageArticle struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	URL         string `json:"url"`
	PictureURL  string `json:"picurl"`
}

type WxWorkAppMpNewsMessageArticle struct {
	Title            string `json:"title"`
	ThumbMediaID     string `json:"thumb_media_id"`
	Author           string `json:"author"`
	ContentSourceURL string `json:"content_source_url"`
	Content          string `json:"content"`
	Digest           string `json:"digest"`
}

type WxWorkAppMiniProgramNoticeMessageItem struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type WxWorkAppTaskCardMessageButton struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	ReplaceName string `json:"replace_name"`
	Color       string `json:"color,omitempty"`
	IsBold      bool   `json:"is_bold,omitempty"`
}

type WxWorkApp struct {
	agentID          string
	corpID           string // see doc https://work.weixin.qq.com/api/doc/90000/90135/91039
	corpSecret       string // see doc https://work.weixin.qq.com/api/doc/90000/90135/90665#secret
	client           *http.Client
	tokenRefreshLock sync.RWMutex // lock to refresh the access token which can expire in a period of time
	accessToken      string       // cached access token
	expiredAt        time.Time    // token expire time
}

func (r *WxWorkApp) IsAccessTokenExpired() bool {
	return time.Now().After(r.expiredAt)
}

// NewWxWorkApp create a new wxwork app
func NewWxWorkApp(corpID, corpSecret, agentID string) *WxWorkApp {
	return NewWxWorkAppWithTimeout(corpID, corpSecret, agentID, WxWorkAppTimeout)
}

// NewWxWorkAppWithTimeout create a new wxwork app with timeout
func NewWxWorkAppWithTimeout(corpID, corpSecret, agentID string, timeout time.Duration) *WxWorkApp {
	client := http.Client{}
	client.Timeout = timeout
	return &WxWorkApp{corpID: corpID, corpSecret: corpSecret, agentID: agentID, client: &client, tokenRefreshLock: sync.RWMutex{}}
}

// NewWxWorkAppWithClient create a new wxwork app with http.Client
func NewWxWorkAppWithClient(corpID, corpSecret, agentID string, client *http.Client) *WxWorkApp {
	return &WxWorkApp{corpID: corpID, corpSecret: corpSecret, agentID: agentID, client: client, tokenRefreshLock: sync.RWMutex{}}
}

func (r *WxWorkApp) SendTextMessage(userIDList []string, partyIDList []string, tagIDList []string, content string,
	options *WxWorkAppMessageSendOptions) (resp WxWorkAppMessageResp, err error) {
	messageObj := make(map[string]interface{})
	messageObj["touser"] = strings.Join(userIDList, "|")
	messageObj["toparty"] = strings.Join(partyIDList, "|")
	messageObj["totag"] = strings.Join(tagIDList, "|")
	messageObj["msgtype"] = WxWorkAppMessageTypeText
	messageObj["agentid"] = r.agentID
	messageObj["text"] = map[string]string{
		"content": content,
	}
	// add options if specified
	if options != nil {
		if options.Safe {
			messageObj["safe"] = 1
		}
		if options.EnableIDTrans {
			messageObj["enable_id_trans"] = 1
		}
		if options.EnableDuplicateCheck {
			messageObj["enable_duplicate_check"] = 1
		}
		if options.DuplicateCheckInterval > 0 {
			messageObj["duplicate_check_interval"] = options.DuplicateCheckInterval
		}
	}
	return r.sendMessage(&messageObj)
}

func (r *WxWorkApp) SendMarkdownMessage(userIDList []string, partyIDList []string, tagIDList []string, content string,
	options *WxWorkAppMessageSendOptions) (resp WxWorkAppMessageResp, err error) {
	messageObj := make(map[string]interface{})
	messageObj["touser"] = strings.Join(userIDList, "|")
	messageObj["toparty"] = strings.Join(partyIDList, "|")
	messageObj["totag"] = strings.Join(tagIDList, "|")
	messageObj["msgtype"] = WxWorkAppMessageTypeMarkdown
	messageObj["agentid"] = r.agentID
	messageObj["markdown"] = map[string]string{
		"content": content,
	}
	// add options if specified
	if options != nil {
		if options.Safe {
			messageObj["safe"] = 1
		}
		if options.EnableIDTrans {
			messageObj["enable_id_trans"] = 1
		}
		if options.EnableDuplicateCheck {
			messageObj["enable_duplicate_check"] = 1
		}
		if options.DuplicateCheckInterval > 0 {
			messageObj["duplicate_check_interval"] = options.DuplicateCheckInterval
		}
	}
	return r.sendMessage(&messageObj)
}

func (r *WxWorkApp) SendImageMessage(userIDList []string, partyIDList []string, tagIDList []string, mediaID string,
	options *WxWorkAppMessageSendOptions) (resp WxWorkAppMessageResp, err error) {
	messageObj := make(map[string]interface{})
	messageObj["touser"] = strings.Join(userIDList, "|")
	messageObj["toparty"] = strings.Join(partyIDList, "|")
	messageObj["totag"] = strings.Join(tagIDList, "|")
	messageObj["msgtype"] = WxWorkAppMessageTypeImage
	messageObj["agentid"] = r.agentID
	messageObj["image"] = map[string]string{
		"media_id": mediaID,
	}
	// add options if specified
	if options != nil {
		if options.Safe {
			messageObj["safe"] = 1
		}
		if options.EnableIDTrans {
			messageObj["enable_id_trans"] = 1
		}
		if options.EnableDuplicateCheck {
			messageObj["enable_duplicate_check"] = 1
		}
		if options.DuplicateCheckInterval > 0 {
			messageObj["duplicate_check_interval"] = options.DuplicateCheckInterval
		}
	}
	return r.sendMessage(&messageObj)
}

func (r *WxWorkApp) SendVoiceMessage(userIDList []string, partyIDList []string, tagIDList []string, mediaID string,
	options *WxWorkAppMessageSendOptions) (resp WxWorkAppMessageResp, err error) {
	messageObj := make(map[string]interface{})
	messageObj["touser"] = strings.Join(userIDList, "|")
	messageObj["toparty"] = strings.Join(partyIDList, "|")
	messageObj["totag"] = strings.Join(tagIDList, "|")
	messageObj["msgtype"] = WxWorkAppMessageTypeVoice
	messageObj["agentid"] = r.agentID
	messageObj["voice"] = map[string]string{
		"media_id": mediaID,
	}
	// add options if specified
	if options != nil {
		if options.Safe {
			messageObj["safe"] = 1
		}
		if options.EnableIDTrans {
			messageObj["enable_id_trans"] = 1
		}
		if options.EnableDuplicateCheck {
			messageObj["enable_duplicate_check"] = 1
		}
		if options.DuplicateCheckInterval > 0 {
			messageObj["duplicate_check_interval"] = options.DuplicateCheckInterval
		}
	}
	return r.sendMessage(&messageObj)
}

func (r *WxWorkApp) SendVideoMessage(userIDList []string, partyIDList []string, tagIDList []string, mediaID, mediaTitle, mediaDescription string,
	options *WxWorkAppMessageSendOptions) (resp WxWorkAppMessageResp, err error) {
	messageObj := make(map[string]interface{})
	messageObj["touser"] = strings.Join(userIDList, "|")
	messageObj["toparty"] = strings.Join(partyIDList, "|")
	messageObj["totag"] = strings.Join(tagIDList, "|")
	messageObj["msgtype"] = WxWorkAppMessageTypeVideo
	messageObj["agentid"] = r.agentID
	messageObj["video"] = map[string]string{
		"media_id":    mediaID,
		"title":       mediaTitle,
		"description": mediaDescription,
	}
	// add options if specified
	if options != nil {
		if options.Safe {
			messageObj["safe"] = 1
		}
		if options.EnableIDTrans {
			messageObj["enable_id_trans"] = 1
		}
		if options.EnableDuplicateCheck {
			messageObj["enable_duplicate_check"] = 1
		}
		if options.DuplicateCheckInterval > 0 {
			messageObj["duplicate_check_interval"] = options.DuplicateCheckInterval
		}
	}
	return r.sendMessage(&messageObj)
}

func (r *WxWorkApp) SendFileMessage(userIDList []string, partyIDList []string, tagIDList []string, mediaID string,
	options *WxWorkAppMessageSendOptions) (resp WxWorkAppMessageResp, err error) {
	messageObj := make(map[string]interface{})
	messageObj["touser"] = strings.Join(userIDList, "|")
	messageObj["toparty"] = strings.Join(partyIDList, "|")
	messageObj["totag"] = strings.Join(tagIDList, "|")
	messageObj["msgtype"] = WxWorkAppMessageTypeFile
	messageObj["agentid"] = r.agentID
	messageObj["file"] = map[string]string{
		"media_id": mediaID,
	}
	// add options if specified
	if options != nil {
		if options.Safe {
			messageObj["safe"] = 1
		}
		if options.EnableIDTrans {
			messageObj["enable_id_trans"] = 1
		}
		if options.EnableDuplicateCheck {
			messageObj["enable_duplicate_check"] = 1
		}
		if options.DuplicateCheckInterval > 0 {
			messageObj["duplicate_check_interval"] = options.DuplicateCheckInterval
		}
	}
	return r.sendMessage(&messageObj)
}

func (r *WxWorkApp) SendTextCardMessage(userIDList []string, partyIDList []string, tagIDList []string, title, description, url, btnText string,
	options *WxWorkAppMessageSendOptions) (resp WxWorkAppMessageResp, err error) {
	messageObj := make(map[string]interface{})
	messageObj["touser"] = strings.Join(userIDList, "|")
	messageObj["toparty"] = strings.Join(partyIDList, "|")
	messageObj["totag"] = strings.Join(tagIDList, "|")
	messageObj["msgtype"] = WxWorkAppMessageTypeTextCard
	messageObj["agentid"] = r.agentID
	messageObj["textcard"] = map[string]string{
		"title":       title,
		"description": description,
		"url":         url,
		"btntxt":      btnText,
	}
	// add options if specified
	if options != nil {
		if options.Safe {
			messageObj["safe"] = 1
		}
		if options.EnableIDTrans {
			messageObj["enable_id_trans"] = 1
		}
		if options.EnableDuplicateCheck {
			messageObj["enable_duplicate_check"] = 1
		}
		if options.DuplicateCheckInterval > 0 {
			messageObj["duplicate_check_interval"] = options.DuplicateCheckInterval
		}
	}
	return r.sendMessage(&messageObj)
}

func (r *WxWorkApp) SendNewsMessage(userIDList []string, partyIDList []string, tagIDList []string, articles []WxWorkAppNewsMessageArticle,
	options *WxWorkAppMessageSendOptions) (resp WxWorkAppMessageResp, err error) {
	messageObj := make(map[string]interface{})
	messageObj["touser"] = strings.Join(userIDList, "|")
	messageObj["toparty"] = strings.Join(partyIDList, "|")
	messageObj["totag"] = strings.Join(tagIDList, "|")
	messageObj["msgtype"] = WxWorkAppMessageTypeNews
	messageObj["agentid"] = r.agentID
	messageObj["news"] = map[string]interface{}{
		"articles": articles,
	}
	// add options if specified
	if options != nil {
		if options.Safe {
			messageObj["safe"] = 1
		}
		if options.EnableIDTrans {
			messageObj["enable_id_trans"] = 1
		}
		if options.EnableDuplicateCheck {
			messageObj["enable_duplicate_check"] = 1
		}
		if options.DuplicateCheckInterval > 0 {
			messageObj["duplicate_check_interval"] = options.DuplicateCheckInterval
		}
	}
	return r.sendMessage(&messageObj)
}

func (r *WxWorkApp) SendMpNewsMessage(userIDList []string, partyIDList []string, tagIDList []string, articles []WxWorkAppMpNewsMessageArticle,
	options *WxWorkAppMessageSendOptions) (resp WxWorkAppMessageResp, err error) {
	messageObj := make(map[string]interface{})
	messageObj["touser"] = strings.Join(userIDList, "|")
	messageObj["toparty"] = strings.Join(partyIDList, "|")
	messageObj["totag"] = strings.Join(tagIDList, "|")
	messageObj["msgtype"] = WxWorkAppMessageTypeMpNews
	messageObj["agentid"] = r.agentID
	messageObj["mpnews"] = map[string]interface{}{
		"articles": articles,
	}
	// add options if specified
	if options != nil {
		if options.Safe {
			messageObj["safe"] = 1
		}
		if options.EnableIDTrans {
			messageObj["enable_id_trans"] = 1
		}
		if options.EnableDuplicateCheck {
			messageObj["enable_duplicate_check"] = 1
		}
		if options.DuplicateCheckInterval > 0 {
			messageObj["duplicate_check_interval"] = options.DuplicateCheckInterval
		}
	}
	return r.sendMessage(&messageObj)
}

func (r *WxWorkApp) SendMiniProgramNoticeMessage(userIDList []string, partyIDList []string, tagIDList []string, appID, page, title, description string,
	emphisFirstItem bool, contentItems []WxWorkAppMiniProgramNoticeMessageItem, options *WxWorkAppMessageSendOptions) (resp WxWorkAppMessageResp, err error) {
	messageObj := make(map[string]interface{})
	messageObj["touser"] = strings.Join(userIDList, "|")
	messageObj["toparty"] = strings.Join(partyIDList, "|")
	messageObj["totag"] = strings.Join(tagIDList, "|")
	messageObj["msgtype"] = WxWorkAppMessageTypeMiniProgramNotice
	messageObj["agentid"] = r.agentID
	messageObj["miniprogram_notice"] = map[string]interface{}{
		"appid":               appID,
		"page":                page,
		"title":               title,
		"description":         description,
		"emphasis_first_item": emphisFirstItem,
		"content_item":        contentItems,
	}
	// add options if specified
	if options != nil {
		if options.Safe {
			messageObj["safe"] = 1
		}
		if options.EnableIDTrans {
			messageObj["enable_id_trans"] = 1
		}
		if options.EnableDuplicateCheck {
			messageObj["enable_duplicate_check"] = 1
		}
		if options.DuplicateCheckInterval > 0 {
			messageObj["duplicate_check_interval"] = options.DuplicateCheckInterval
		}
	}
	return r.sendMessage(&messageObj)
}

func (r *WxWorkApp) SendTaskCardMessage(userIDList []string, partyIDList []string, tagIDList []string, taskID, title, description, url string,
	buttons []WxWorkAppTaskCardMessageButton, options *WxWorkAppMessageSendOptions) (resp WxWorkAppMessageResp, err error) {
	messageObj := make(map[string]interface{})
	messageObj["touser"] = strings.Join(userIDList, "|")
	messageObj["toparty"] = strings.Join(partyIDList, "|")
	messageObj["totag"] = strings.Join(tagIDList, "|")
	messageObj["msgtype"] = WxWorkAppMessageTypeTaskCard
	messageObj["agentid"] = r.agentID
	messageObj["taskcard"] = map[string]interface{}{
		"task_id":     taskID,
		"title":       title,
		"description": description,
		"url":         url,
		"btn":         buttons,
	}
	// add options if specified
	if options != nil {
		if options.Safe {
			messageObj["safe"] = 1
		}
		if options.EnableIDTrans {
			messageObj["enable_id_trans"] = 1
		}
		if options.EnableDuplicateCheck {
			messageObj["enable_duplicate_check"] = 1
		}
		if options.DuplicateCheckInterval > 0 {
			messageObj["duplicate_check_interval"] = options.DuplicateCheckInterval
		}
	}
	return r.sendMessage(&messageObj)
}

// See doc https://work.weixin.qq.com/api/doc/90000/90135/90236
func (r *WxWorkApp) sendMessage(messageObj interface{}) (messageResp WxWorkAppMessageResp, err error) {
	err = r.fireRequest(http.MethodPost, WxWorkAppMessageAPI, nil, messageObj, &messageResp)
	if err != nil {
		return
	}
	if messageResp.ErrCode != WxWorkAppStatusOK {
		if messageResp.ErrCode == WxWorkCodeAccessTokenExpired {
			// reset the access token
			r.accessToken = ""
		}
		err = fmt.Errorf("call wxwork app message api error, %d %s", messageResp.ErrCode, messageResp.ErrMessage)
		return
	}
	return
}

// CreateGroupChat create a new group chat
func (r *WxWorkApp) CreateGroupChat(name, ownerID string, userIDList []string, options *WxWorkAppCreateGroupOptions) (newChatID string, err error) {
	createGroupReqObject := make(map[string]interface{})
	createGroupReqObject["name"] = name
	createGroupReqObject["owner"] = ownerID
	createGroupReqObject["userlist"] = userIDList
	if options != nil {
		createGroupReqObject["chatid"] = options.ChatID
	}
	var createGroupResp WxWorkAppCreateGroupResp
	err = r.fireRequest(http.MethodPost, WxWorkAppCreateGroupAPI, nil, &createGroupReqObject, &createGroupResp)
	if err != nil {
		return
	}
	if createGroupResp.ErrCode != WxWorkAppStatusOK {
		if createGroupResp.ErrCode == WxWorkCodeAccessTokenExpired {
			// reset the access token
			r.accessToken = ""
		}
		err = fmt.Errorf("call wxwork app create group api error, %d %s", createGroupResp.ErrCode, createGroupResp.ErrMessage)
		return
	}
	newChatID = createGroupResp.ChatID
	return
}

func (r *WxWorkApp) UpdateGroupChat(chatID string, options *WxWorkAppUpdateGroupOptions) (err error) {
	updateGroupReqObject := make(map[string]interface{})
	updateGroupReqObject["chatid"] = chatID
	if options != nil {
		updateGroupReqObject["name"] = options.Name
		updateGroupReqObject["owner"] = options.Owner
		updateGroupReqObject["add_user_list"] = options.AddUserList
		updateGroupReqObject["del_user_list"] = options.DelUserList
	}
	var updateGroupResp WxWorkAppUpdateGroupResp
	err = r.fireRequest(http.MethodPost, WxWorkAppUpdateGroupAPI, nil, &updateGroupReqObject, &updateGroupResp)
	if err != nil {
		return
	}
	if updateGroupResp.ErrCode != WxWorkAppStatusOK {
		if updateGroupResp.ErrCode == WxWorkCodeAccessTokenExpired {
			// reset the access token
			r.accessToken = ""
		}
		err = fmt.Errorf("call wxwork app update group api error, %d %s", updateGroupResp.ErrCode, updateGroupResp.ErrMessage)
		return
	}
	return
}

func (r *WxWorkApp) GetGroupChat(chatID string) (group WxWorkAppGroup, err error) {
	var getGroupResp WxWorkAppGetGroupResp
	err = r.fireRequest(http.MethodGet, WxWorkAppGetGroupAPI, map[string]string{"chatid": chatID}, nil, &getGroupResp)
	if err != nil {
		return
	}
	if getGroupResp.ErrCode != WxWorkAppStatusOK {
		if getGroupResp.ErrCode == WxWorkCodeAccessTokenExpired {
			// reset the access token
			r.accessToken = ""
		}
		err = fmt.Errorf("call wxwork app get group api error, %d %s", getGroupResp.ErrCode, getGroupResp.ErrMessage)
		return
	}
	group = getGroupResp.ChatInfo
	return
}

func (r *WxWorkApp) SendGroupTextMessage(chatID, content string, options *WxWorkAppMessageSendOptions) (err error) {
	messageObj := make(map[string]interface{})
	messageObj["chatid"] = chatID
	messageObj["msgtype"] = WxWorkAppMessageTypeText
	messageObj["text"] = map[string]string{
		"content": content,
	}
	if options != nil && options.Safe {
		messageObj["safe"] = 1
	}
	return r.sendGroupMessage(&messageObj)
}

func (r *WxWorkApp) SendGroupMarkdownMessage(chatID, content string, options *WxWorkAppMessageSendOptions) (err error) {
	messageObj := make(map[string]interface{})
	messageObj["chatid"] = chatID
	messageObj["msgtype"] = WxWorkAppMessageTypeMarkdown
	messageObj["markdown"] = map[string]string{
		"content": content,
	}
	if options != nil && options.Safe {
		messageObj["safe"] = 1
	}
	return r.sendGroupMessage(&messageObj)
}

func (r *WxWorkApp) SendGroupImageMessage(chatID, mediaID string, options *WxWorkAppMessageSendOptions) (err error) {
	messageObj := make(map[string]interface{})
	messageObj["chatid"] = chatID
	messageObj["msgtype"] = WxWorkAppMessageTypeImage
	messageObj["image"] = map[string]string{
		"media_id": mediaID,
	}
	if options != nil && options.Safe {
		messageObj["safe"] = 1
	}
	return r.sendGroupMessage(&messageObj)
}

func (r *WxWorkApp) SendGroupVoiceMessage(chatID, mediaID string, options *WxWorkAppMessageSendOptions) (err error) {
	messageObj := make(map[string]interface{})
	messageObj["chatid"] = chatID
	messageObj["msgtype"] = WxWorkAppMessageTypeVoice
	messageObj["voice"] = map[string]string{
		"media_id": mediaID,
	}
	if options != nil && options.Safe {
		messageObj["safe"] = 1
	}
	return r.sendGroupMessage(&messageObj)
}

func (r *WxWorkApp) SendGroupVideoMessage(chatID, mediaID, mediaTitle, mediaDescription string, options *WxWorkAppMessageSendOptions) (err error) {
	messageObj := make(map[string]interface{})
	messageObj["chatid"] = chatID
	messageObj["msgtype"] = WxWorkAppMessageTypeVideo
	messageObj["video"] = map[string]string{
		"media_id":    mediaID,
		"title":       mediaTitle,
		"description": mediaDescription,
	}
	if options != nil && options.Safe {
		messageObj["safe"] = 1
	}
	return r.sendGroupMessage(&messageObj)
}

func (r *WxWorkApp) SendGroupFileMessage(chatID, mediaID string, options *WxWorkAppMessageSendOptions) (err error) {
	messageObj := make(map[string]interface{})
	messageObj["chatid"] = chatID
	messageObj["msgtype"] = WxWorkAppMessageTypeFile
	messageObj["file"] = map[string]string{
		"media_id": mediaID,
	}
	if options != nil && options.Safe {
		messageObj["safe"] = 1
	}
	return r.sendGroupMessage(&messageObj)
}

func (r *WxWorkApp) SendGroupTextCardMessage(chatID, title, description, url, btnText string, options *WxWorkAppMessageSendOptions) (err error) {
	messageObj := make(map[string]interface{})
	messageObj["chatid"] = chatID
	messageObj["msgtype"] = WxWorkAppMessageTypeTextCard
	messageObj["textcard"] = map[string]string{
		"title":       title,
		"description": description,
		"url":         url,
		"btntext":     btnText,
	}
	if options != nil && options.Safe {
		messageObj["safe"] = 1
	}
	return r.sendGroupMessage(&messageObj)
}

func (r *WxWorkApp) SendGroupNewsMessage(chatID string, articles []WxWorkAppNewsMessageArticle, options *WxWorkAppMessageSendOptions) (err error) {
	messageObj := make(map[string]interface{})
	messageObj["chatid"] = chatID
	messageObj["msgtype"] = WxWorkAppMessageTypeNews
	messageObj["news"] = map[string]interface{}{
		"articles": articles,
	}
	if options != nil && options.Safe {
		messageObj["safe"] = 1
	}
	return r.sendGroupMessage(&messageObj)
}

func (r *WxWorkApp) SendGroupMpNewsMessage(chatID string, articles []WxWorkAppMpNewsMessageArticle, options *WxWorkAppMessageSendOptions) (err error) {
	messageObj := make(map[string]interface{})
	messageObj["chatid"] = chatID
	messageObj["msgtype"] = WxWorkAppMessageTypeMpNews
	messageObj["mpnews"] = map[string]interface{}{
		"articles": articles,
	}
	if options != nil && options.Safe {
		messageObj["safe"] = 1
	}
	return r.sendGroupMessage(&messageObj)
}

func (r *WxWorkApp) refreshAccessToken() (err error) {
	reqURL := fmt.Sprintf("%s?corpid=%s&corpsecret=%s", WxWorkAppTokenAPI, r.corpID, r.corpSecret)
	req, newErr := http.NewRequest(http.MethodGet, reqURL, nil)
	if newErr != nil {
		err = fmt.Errorf("create request error, %s", newErr.Error())
		return
	}
	resp, getErr := r.client.Do(req)
	if getErr != nil {
		err = fmt.Errorf("get response error, %s", getErr.Error())
		return
	}
	defer resp.Body.Close()
	// check http code
	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("wxwork request error, %s", resp.Status)
		io.Copy(ioutil.Discard, resp.Body)
		return
	}
	// parse response body
	decoder := json.NewDecoder(resp.Body)
	var wxTokenResp WxWorkAppTokenResp
	if decodeErr := decoder.Decode(&wxTokenResp); decodeErr != nil {
		err = fmt.Errorf("parse response error, %s", decodeErr.Error())
		return
	}
	if wxTokenResp.ErrCode != WxWorkAppStatusOK {
		err = fmt.Errorf("call wxwork app api error, %d %s", wxTokenResp.ErrCode, wxTokenResp.ErrMessage)
		return
	}
	// set access token and expired at
	r.accessToken = wxTokenResp.AccessToken
	r.expiredAt = time.Now().Add(time.Second * time.Duration(wxTokenResp.ExpiresIn))
	return
}

// See doc https://work.weixin.qq.com/api/doc/90000/90135/90248
func (r *WxWorkApp) sendGroupMessage(messageObj interface{}) (err error) {
	var messageResp WxWorkAppGroupMessageResp
	err = r.fireRequest(http.MethodPost, WxWorkAppGroupMessageAPI, nil, messageObj, &messageResp)
	if err != nil {
		return
	}
	if messageResp.ErrCode != WxWorkAppStatusOK {
		if messageResp.ErrCode == WxWorkCodeAccessTokenExpired {
			// reset the access token
			r.accessToken = ""
		}
		err = fmt.Errorf("call wxwork app group message api error, %d %s", messageResp.ErrCode, messageResp.ErrMessage)
		return
	}
	return
}

func (r *WxWorkApp) UploadMedia(fileBody []byte, fileName, fileType string) (mediaID string, createdAt int64, err error) {
	var uploadMediaResp WxWorkAppUploadMediaResp
	err = r.uploadFile(http.MethodPost, WxWorkAppUploadMediaAPI, map[string]string{"type": fileType}, fileBody, fileName, &uploadMediaResp)
	if err != nil {
		return
	}
	if uploadMediaResp.ErrCode != WxWorkAppStatusOK {
		if uploadMediaResp.ErrCode == WxWorkCodeAccessTokenExpired {
			// reset the access token
			r.accessToken = ""
		}
		err = fmt.Errorf("call wxwork app upload media api error, %d %s", uploadMediaResp.ErrCode, uploadMediaResp.ErrMessage)
		return
	}
	// set fields
	mediaID = uploadMediaResp.MediaID
	createdAt, _ = strconv.ParseInt(uploadMediaResp.CreatedAt, 10, 64)
	return
}

func (r *WxWorkApp) UploadImage(fileBody []byte, fileName string) (imageURL string, err error) {
	var uploadImageResp WxWorkAppUploadImageResp
	err = r.uploadFile(http.MethodPost, WxWorkAppUploadImageAPI, nil, fileBody, fileName, &uploadImageResp)
	if err != nil {
		return
	}
	if uploadImageResp.ErrCode != WxWorkAppStatusOK {
		if uploadImageResp.ErrCode == WxWorkCodeAccessTokenExpired {
			// reset the access token
			r.accessToken = ""
		}
		err = fmt.Errorf("call wxwork app upload image api error, %d %s", uploadImageResp.ErrCode, uploadImageResp.ErrMessage)
		return
	}
	// set fields
	imageURL = uploadImageResp.URL
	return
}

func (r *WxWorkApp) uploadFile(reqMethod, reqURL string, reqParams map[string]string, fileBody []byte, fileName string, wxUploadFileResp interface{}) (err error) {
	// check the token expired or not
	if r.accessToken == "" || r.IsAccessTokenExpired() {
		r.tokenRefreshLock.Lock()
		if r.accessToken == "" || r.IsAccessTokenExpired() {
			err = r.refreshAccessToken()
		}
		r.tokenRefreshLock.Unlock()
		if err != nil {
			err = fmt.Errorf("refresh access token error, %s", err.Error())
			return
		}
	}
	// check params
	queryString := url.Values{}
	queryString.Add("access_token", r.accessToken)
	if reqParams != nil {
		for k, v := range reqParams {
			queryString.Add(k, v)
		}
	}

	reqURL = fmt.Sprintf("%s?%s", reqURL, queryString.Encode())
	// create body
	respBodyBuffer := bytes.NewBuffer(nil)
	defer respBodyBuffer.Reset()
	multipartWriter := multipart.NewWriter(respBodyBuffer)
	// add form data
	formFileWriter, createErr := multipartWriter.CreateFormFile("media", fileName)
	if createErr != nil {
		err = fmt.Errorf("create form file error, %s", createErr.Error())
		return
	}
	if _, writeErr := formFileWriter.Write(fileBody); writeErr != nil {
		err = fmt.Errorf("write form file error, %s", writeErr.Error())
		return
	}
	if closeErr := multipartWriter.Close(); closeErr != nil {
		err = fmt.Errorf("close form file error, %s", closeErr.Error())
		return
	}
	// create new request
	req, newErr := http.NewRequest(reqMethod, reqURL, respBodyBuffer)
	if newErr != nil {
		err = fmt.Errorf("create request error, %s", newErr.Error())
		return
	}
	// set multi-part header
	req.Header.Set("Content-Type", multipartWriter.FormDataContentType())
	resp, getErr := r.client.Do(req)
	if getErr != nil {
		err = fmt.Errorf("get response error, %s", getErr.Error())
		return
	}
	defer resp.Body.Close()
	// check http code
	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("wxwork app request error, %s", resp.Status)
		io.Copy(ioutil.Discard, resp.Body)
		return
	}
	// parse response body
	decoder := json.NewDecoder(resp.Body)
	if decodeErr := decoder.Decode(&wxUploadFileResp); decodeErr != nil {
		err = fmt.Errorf("parse response error, %s", decodeErr.Error())
		return
	}
	return
}

func (r *WxWorkApp) fireRequest(reqMethod, reqURL string, reqParams map[string]string, reqBodyObject interface{}, respObject interface{}) (err error) {
	// check the token expired or not
	if r.accessToken == "" || r.IsAccessTokenExpired() {
		r.tokenRefreshLock.Lock()
		if r.accessToken == "" || r.IsAccessTokenExpired() {
			err = r.refreshAccessToken()
		}
		r.tokenRefreshLock.Unlock()
		if err != nil {
			err = fmt.Errorf("refresh access token error, %s", err.Error())
			return
		}
	}

	queryString := url.Values{}
	queryString.Add("access_token", r.accessToken)
	if reqParams != nil {
		for k, v := range reqParams {
			queryString.Add(k, v)
		}
	}

	reqURL = fmt.Sprintf("%s?%s", reqURL, queryString.Encode())
	var reqBodyReader io.Reader
	if reqBodyObject != nil {
		reqBody, _ := json.Marshal(reqBodyObject)
		reqBodyReader = bytes.NewReader(reqBody)
	}

	req, newErr := http.NewRequest(reqMethod, reqURL, reqBodyReader)
	if newErr != nil {
		err = fmt.Errorf("create request error, %s", newErr.Error())
		return
	}
	req.Header.Add("Content-Type", "application/json")
	resp, getErr := r.client.Do(req)
	if getErr != nil {
		err = fmt.Errorf("get response error, %s", getErr.Error())
		return
	}
	defer resp.Body.Close()
	// check http code
	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("wxwork request error, %s", resp.Status)
		io.Copy(ioutil.Discard, resp.Body)
		return
	}
	// parse response body
	decoder := json.NewDecoder(resp.Body)
	if decodeErr := decoder.Decode(respObject); decodeErr != nil {
		err = fmt.Errorf("parse response error, %s", decodeErr.Error())
		return
	}
	return
}
