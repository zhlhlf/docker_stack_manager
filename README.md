# Docker Stack Manager

杞婚噺绾?Docker Swarm Stack 鏈嶅姟绠＄悊闈㈡澘銆?
## 鍔熻兘
- Stack 涓庣鍙ｇ櫧鍚嶅崟绠＄悊
- **鐪熷疄 Docker Swarm Service** 褰掑睘/绔彛杩濊妫€娴?- 涓€閿竻鐞?/ 瀹氭椂鑷姩娓呯悊
- 鍓嶇 `go:embed` 鎵撹繘鍗曚簩杩涘埗
- x-ui 椋庢牸 Web UI

## 杩愯锛堥渶瑕佸彲璁块棶 Docker Engine锛?
```bash
# 鏈満锛圠inux / 宸茶 Docker锛?go run .
# 鎴?./docker_stack_manager_linux_amd64 -addr :8080
```

娴忚鍣細http://localhost:8080

### 瀹瑰櫒杩愯锛堟帹鑽愶級

```bash
docker run -d --name stack-manager \
  -p 8080:8080 \
  -v /var/run/docker.sock:/var/run/docker.sock \
  -v stack-manager-data:/data \
  ghcr.io/zhlhlf/docker_stack_manager:latest
```

> 蹇呴』鎸傝浇 Docker socket锛岀▼搴忛€氳繃 Engine API 璇诲彇/鍒犻櫎 Swarm Service銆?
### 鍙傛暟 / 鐜鍙橀噺
- `-addr` / `LISTEN_ADDR`  榛樿 `:8080`
- `-db` / `DB_PATH`        榛樿 `data.json`

### 鏉冮檺
Docker 闇€瑕侊細
- `ServiceList`
- `ServiceInspect`锛堝垪琛ㄥ凡鍚級
- `ServiceRemove`

寤鸿璺戝湪 Swarm manager 鑺傜偣銆?
## 杩濊瑙勫垯
1. 鏃犳硶褰掑睘鍒板凡閰嶇疆 Stack  
   - 浼樺厛鏍囩 `com.docker.stack.namespace`  
   - 鍏舵鏈嶅姟鍚嶅墠缂€ `<stack>_<service>` 涓?Stack 宸查厤缃?2. 宸插綊灞?Stack锛屼絾**鍙戝竷绔彛**涓嶅湪鐧藉悕鍗? 
   - 鐧藉悕鍗曚负绌?= 涓嶅厑璁镐换浣曞彂甯冪鍙?3. 鏃犲彂甯冪鍙ｇ殑鏈嶅姟涓嶅垽瀹氱鍙ｈ繚瑙?
## CI
- push `main` 瑙﹀彂 Release
- tag = `v` + commit 鍓?8 浣?- 浠呮瀯寤?Linux amd64