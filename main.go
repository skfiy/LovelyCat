package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"github.com/larspensjo/config"

	"github.com/bitly/go-simplejson"
)

// GoURL 当前地址
const GoURL = "http://127.0.0.1:8081"

// ReceiveURL 接收地址
const ReceiveURL = "http://127.0.0.1:8073/send"

var (
	types         int    // @var int 事件类型<br><br> 100=> 私聊消息<br>200=> 群聊消息<br>300=> 暂无<br>400=> 群成员增加<br>410=> 群成员减少<br>500=> 收到好友请求<br>600=> 二维码收款<br>700=> 收到转账<br>800=> 软件开始启动<br>900=> 新的账号登录完成<br>910=> 账号下线
	fromWxid      string // @var string 1级来源id（群消息事件下，1级来源为群id，2级来源为发消息的成员id，私聊事件下都一样）
	fromName      string // @var string 1级来源昵称（比如发消息的人昵称）
	finalFromWxid string // @var string 2级来源id（群消息事件下，1级来源为群id，2级来源为发消息的成员id，私聊事件下都一样）
	finalFromName string // @var string 2级来源昵称
	robotWxid     string // @var string 当前登录的账号（机器人）标识id
	msg           string // @var string 消息内容
	msgType       int    // @var int 消息类型（请务必使用新版http插件）<br><br> 1 =>文本消息 <br>3 => 图片消息 <br>34 => 语音消息 <br>42 => 名片消息 <br>43 =>视频 <br>47 => 动态表情 <br> 48 =>地理位置<br>49 => 分享链接 <br>2001 => 红包<br>2002 => 小程序<br>2003 => 群邀请 <br><br>更多请参考sdk模块常量值
	fileUrls      string // @var string 如果是文件消息（图片、语音、视频、动态表情），这里则是可直接访问的网络地址，非文件消息时为空
	times         int    // @var int 请求时间(时间戳10位版本)
)

// wxData 消息文本
type wxData struct {
	Type      int    `json:"type"`
	RobotWxid string `json:"robot_wxid"`
	Msg       string `json:"msg"`
	Towxid    string `json:"to_wxid"`
	//GroupWxid  string `json:"group_wxid"`
	//FriendWxid string `json:"friend_wxid"`
	//IsRefresh int `json:"is_refresh"`
}

// wxGroupData 获取群聊列表
type wxGroupData struct {
	Type      int    `json:"type"`
	RobotWxid string `json:"robot_wxid"`
	//Msg       string `json:"msg"`
	//Towxid    string `json:"to_wxid"`
	//GroupWxid  string `json:"group_wxid"`
	//FriendWxid string `json:"friend_wxid"`
	IsRefresh int `json:"is_refresh"`
}

// wxImgData 消息图片
type wxImgData struct {
	Type      int    `json:"type"`
	RobotWxid string `json:"robot_wxid"`
	Msg       string `json:"msg"`
	Towxid    string `json:"to_wxid"`
}

// JdJSONText  京东短连接获取
type JdJSONText struct {
	Code int    `json:"code"`
	Data jdData `json:"data"`
}
type jdData struct {
	ShortURL  string `json:"shortURL"`
	Message   string `json:"message"`
	RequestID string `json:"requestId"`
}

//ServiceError 自定义错误
type ServiceError struct {
	Msg string
}

func (e *ServiceError) Error() string {
	return fmt.Sprintf("%s", e.Msg)
}

//NewServiceError 自定义错误func
func NewServiceError(msg string) error {
	return &ServiceError{msg}
}

func httpStart() {

	http.HandleFunc("/api", handlePostJSON)
	//http.HandleFunc("/group", getGroupList)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "%s", "aaaa")
	})
	go func() {
		for {
			time.Sleep(time.Second)
			log.Println("Checking if started...")
			resp, err := http.Get(GoURL)
			if err != nil {
				log.Println("Failed:", err)
				continue
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				log.Println("Not OK:", resp.StatusCode)
				continue
			}
			break
		}
		log.Println("SERVER 启动成功!")
		log.Println("URL：", GoURL)
	}()

	err := http.ListenAndServe(":8081", nil) // 设置监听的端口
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}

}

// handlePostJSON  Post接收参数
func handlePostJSON(w http.ResponseWriter, r *http.Request) {
	// 检查请求
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		fmt.Fprintf(w, "invalid_http_method")
		return
	}
	defer r.Body.Close()
	//获取post的数据
	params, _ := ioutil.ReadAll(r.Body)
	//log.Println("POST请求的type类型----> :", r.Header.Get("Content-Type"))
	//赋值参数
	getURLPostData(string(params))

	var filename = time.Now().Format("2006-01-02") + ".txt"
	//写入日志
	logstring := "事件类型【" + strconv.Itoa(types) + "】,消息类型【" + strconv.Itoa(msgType) + "】,来源【" + fromWxid + "---" + fromName + "】,来源2【" + finalFromWxid + "---" + finalFromName + "】,获取内容【" + msg + "】."
	logInfo(filename, logstring)
	returnMsg()
}

// handleGet GET接收参数
func handleGet(w http.ResponseWriter, r *http.Request) {
	// 检查请求
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		fmt.Fprintf(w, "invalid_http_method")
	}
}

// checkFileIsExist 判断文件是否存在  存在返回 true 不存在返回false
func checkFileIsExist(filename string) bool {
	var exist = true
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		exist = false
	}
	return exist
}

func isCheck(e error) {
	if e != nil {
		log.Println("报错内容 ：", e)
	}
}

//	logInfo 日志
func logInfo(pathName string, content string) {

	var f *os.File
	var err1 error
	//检测文件是否存在
	if checkFileIsExist(pathName) {
		//打开文件
		f, err1 = os.OpenFile(pathName, os.O_APPEND, 0666)
		//log.Println("该文件存在")
	} else {
		//创建文件
		f, err1 = os.Create(pathName)
		log.Println("该文件不存在，创建文件：", pathName)
	}
	isCheck(err1)
	defer f.Close()
	logcontents := time.Now().Format("2006-01-02 15:04:05") + "-->: \r\n" + content + "\r\n--------------------\r\n"
	_, err1 = io.WriteString(f, logcontents)
	isCheck(err1)
	//fmt.Printf("写入 %d 个字节", n)

}

// getURLPostData  赋值
func getURLPostData(d string) {
	//创建map
	params := make(map[string]string)
	//截取内容
	u := strings.Split(d, "&")
	for i := 0; i < len(u); i++ {
		str := u[i]
		if len(str) > 0 {
			tem := strings.Split(str, "=")
			if len(tem) > 0 && len(tem) == 1 {
				params[tem[0]] = ""
			} else if len(tem) > 1 {
				params[tem[0]] = tem[1]
			}
		}
	}
	//fmt.Printf("类型：%T \n", s)
	types, _ = strconv.Atoi(params["type"])
	fromWxid = params["from_wxid"]
	fromName, _ = url.QueryUnescape(params["from_name"])
	finalFromWxid = params["final_from_wxid"]
	finalFromName, _ = url.QueryUnescape(params["final_from_name"])
	robotWxid, _ = url.QueryUnescape(params["robot_wxid"])
	msgType, _ = strconv.Atoi(params["msg_type"])
	msg, _ = url.QueryUnescape(params["msg"])
	fileUrls = params["file_url"]
	times, _ = strconv.Atoi(params["time"])
}

// SimpleHTTPPost POST请求
func SimpleHTTPPost(urlstr string, params interface{}) ([]byte, error) {

	jsonPost, err := json.Marshal(params)
	DataURLVal := url.Values{}
	DataURLVal.Add("data", string(jsonPost))
	logInfo("longInfosend.txt", string(jsonPost))
	if err != nil {
		return []byte(""), errors.New("json encode fail")
	}
	payload := strings.NewReader(DataURLVal.Encode())
	log.Println(payload)
	req, _ := http.NewRequest("POST", urlstr, payload)
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	//req.Header.Add("cache-control", "no-cache")
	res, _ := http.DefaultClient.Do(req)
	body, _ := ioutil.ReadAll(res.Body)
	defer res.Body.Close()

	return body, nil
}

// PostWithFormData 获取京东短连接
func PostWithFormData(method, urlstr string, params map[string]string) (string, error) {
	body := new(bytes.Buffer)
	w := multipart.NewWriter(body)

	// for k, v := range *params {
	// 	fmt.Println("循环获取：", k, v)
	// 	w.WriteField("appkey", v)
	// }
	w.WriteField("appkey", params["appkey"])
	w.WriteField("content", params["content"])
	w.WriteField("jd_lianmeng_id", params["jd_lianmeng_id"])
	w.WriteField("positionId", params["positionId"])
	defer w.Close()
	req, _ := http.NewRequest(method, urlstr, body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp, _ := http.DefaultClient.Do(req)
	data, _ := ioutil.ReadAll(resp.Body)
	defer resp.Body.Close()
	//fmt.Println(resp.StatusCode)
	js, err := simplejson.NewJson([]byte(data))
	fmt.Println(js)
	resultinfo := js.Get("jd_union_open_promotion_byunionid_get_response").Get("result").MustString()
	if err != nil {
		fmt.Println(err)
	}
	d := &JdJSONText{}
	er := json.Unmarshal([]byte(resultinfo), d)
	if er != nil {
		fmt.Println("未获取到URL", er)
	}
	return d.Data.ShortURL, nil
}

// getGroupList 获取当前微信群列表
func getGroupList() {

	wxGroupData := &wxGroupData{
		Type:      205,
		RobotWxid: robotWxid,
		IsRefresh: 1,
	}
	jsonBody, err := SimpleHTTPPost(ReceiveURL, wxGroupData)
	if err != nil {
		fmt.Println(string(jsonBody))
		tname := "grouplist.txt"
		logInfo(tname, string(jsonBody))
	}

}

// sendTextMsg 发送文本消息
func sendTextMsg() {
	tomsg := strRegexp(msg)
	logInfo("longInfosend.txt",tomsg)
	if tomsg != "" {
		wxdata := &wxData{
			Type:      100,
			RobotWxid: robotWxid,
			Msg:       url.QueryEscape(tomsg),
			Towxid:    fromWxid,
		}
		jsonBody, err := SimpleHTTPPost(ReceiveURL, wxdata)
		if err != nil {
			fmt.Println(string(jsonBody))
			tname := "logsend.txt"
			logInfo(tname, string(jsonBody))
		} else {
			log.Println("http发送请求：", err)
		}
	}

}

//  sendImgMsg  发送图片消息
func sendImgMsg() {

	wxdata := &wxImgData{
		Type:      103,
		RobotWxid: robotWxid,
		Msg:       url.QueryEscape(msg),
		Towxid:    fromWxid,
	}
	jsonBody, err := SimpleHTTPPost(ReceiveURL, wxdata)
	if err != nil {
		fmt.Println(string(jsonBody))
		tname := "logsend.txt"
		logInfo(tname, string(jsonBody))
	} else {
		log.Println("http发送请求：", err)
	}
}

// returnMsg  判断  接收的 types 事件类型
func returnMsg() {
	switch types { //finger is declared in switch
	case 100:
		//fmt.Println("Thumb")
	case 200:
		gorupReturnList()
	case 300:
		//fmt.Println("Middle")
	case 400:
		//fmt.Println("Ring")
	case 900:
		//fmt.Println("Pinky")
		//default: //default case
		//fmt.Println("incorrect finger number")
	}
}

func gorupReturnList() {

	switch msgType {
	case 1:
		sendTextMsg()
	case 3:
		sendImgMsg()
	case 47:
		fmt.Println("消息类型47，动态图片---》", msg)
	}
}

// strRegexp 正则匹配多个域名通过联盟ID转换京东短连接
func strRegexp(str string) string {
	furl := "https://api.zhetaoke.com:10001/api/open_jing_zhuanlian.ashx"
	s := regexp.MustCompile(`(ht|f)tp(s?)\:\/\/(u.jd)([-.\w]*[0-9a-zA-Z])*(:(0-9)*)*(\/?)([a-zA-Z0-9\-\.\?\,\'\/\\\+&amp;%\$#_]*)?`)
	to := s.FindAllString(str, -1)
	if len(to) > 0 {
		BASIC, _ := getConfig("BASIC")
		fmt.Println(BASIC)
		for _, jdurl := range to {
			data := make(map[string]string)
			data["appkey"] = BASIC["appkey"]
			data["content"] = jdurl
			data["jd_lianmeng_id"] = BASIC["jd_lianmeng_id"]
			data["positionId"] = BASIC["positionId"]
			jsonBody, err := PostWithFormData("POST", furl, data)
			if err != nil {
				fmt.Println(string(jsonBody))
				tname := "jdurl.txt"
				logInfo(tname, string(jsonBody))
			}
			str = strings.Replace(str, jdurl, jsonBody, -1)
		}
		msg = str
		return msg
	}
	return msg
}



func getConfig(sec string) (map[string]string, error) {
	targetConfig := make(map[string]string)
	cfg, err := config.ReadDefault("Config.ini")
	if err != nil {
		return targetConfig, NewServiceError("unable to open config file or wrong fomart")
	}
	sections := cfg.Sections()
	if len(sections) == 0 {
		return targetConfig, NewServiceError("no " + sec + " config")
	}
	for _, section := range sections {
		if section != sec {
			continue
		}
		sectionData, _ := cfg.SectionOptions(section)
		for _, key := range sectionData {
			value, err := cfg.String(section, key)
			if err == nil {
				targetConfig[key] = value
			}
		}
		break
	}
	return targetConfig, nil
}



func main() {
	httpStart()
}
