<h1 align="center">
  NAS-TikTok
</h1>

<p align="center">
 <a href="README.md">简体中文</a> | <a href="README_en.md">English</a>
</p>

<p align="center">
<a><img src="https://img.shields.io/badge/license-GPLv3-orange"/></a>
<a><img src="https://img.shields.io/badge/PRs-welcome-brightgreen"/></a>
<a><img src="https://img.shields.io/badge/Powered%20by-NestJS-blue"/></a>
<a><img src="https://img.shields.io/badge/Powered%20by-Vue.js-blue"/></a>

</p>

Are you overwhelmed by too many resources on your NAS and don't know what to watch? Now, `NAS-TikTok` is here to solve that problem! `NAS-TikTok` is a project featuring a frontend that mimics `TikTok` ，and a backend that randomly samples your media library. It provides a smooth and fluid experience for Browse your NAS resources, just like scrolling through TikTok!

## Technology Stack

### Backend NestJS

1. Responsible for AUTH authentication.

2. Performs reservoir sampling on the resource library (no database/no cache).

3. Supports HTTPS HTTP2 (requires your own domain/certificate).

4. Supports LAN-only access (Docker requires host mode).

5. Supports client IP binding (even if the URL is leaked, it cannot be viewed if the IP is different).

### Frontend Vue.js

1. Supports random jump to video timestamp (even if you swipe to the same video, a different segment will play).

2. Supports 2x speed playback / video rotation.

3. Supports image viewing / magnification / rotation / autoplay.

4. Operation buttons are hidden by default and only shown on touch, keeping the view unobstructed.

5. Supports cache-free PWA: iOS devices can add it to the home screen from Safari (Android not tested) and use it as an app, with no data cached on the device.

## Online Access

Currently, online access and Docker images are not provided (as I'm unsure if there's interest in this project~).

## Running

Note: This project is for learning and research purposes only and must not be used for commercial purposes.

### Development

---

```bash
git clone https://github.com/ASentimentalMan/NASTikTok
cd NASTikTok

# Create the media folder and place test resources inside
mkdir NASTikTok/statics

# Backend
cd NASTikTok/server
npm install
npm run dev

# Frontend
cd NASTikTok/client
npm install
npm run dev
```

Open your browser and visit: [http://127.0.0.1:3000](http://127.0.0.1:3000)

### Deployment

```bash
# Create a Docker image
docker build -t nas-tiktok .

# If your NAS cannot run Dockerfile, please build and export it from your computer, then import it to your NAS to start
docker save -o nas-tiktok.tar nas-tiktok
```

#### Docker Internal Structure

```bash
/workspace
├── cert/ # Certificate folder
│   ├── fullchain.pem # Certificate
│   └── privkey.pem # Private key
├── static/ # Resource directory
└── ...
```

#### Docker Startup Command

```bash
# Startup parameter description
docker run -d \
  # /cert -> Certificate folder, must contain valid fullchain.pem and privkey.pem
  -v /cert:/workspace/cert \
  # /source -> Resource folder, resources to be browsed should be mapped here
  -v /source:/workspace/statics \
  # port -> Mapped port: only required for bridge mode
  # In bridge mode, Docker cannot get the real IP, so IP change logout and LAN-only access features will not be available
  -p port:port \
  # SSL -> Enable SSL: if enabled, cert folder and certificates must be provided
  -e SSL=false \
  # LAN_ONLY -> Allow LAN-only access
  -e LAN_ONLY=false \
  # ACCOUNT -> Your account
  -e ACCOUNT= \
  # PASSWORD -> Your password
  -e PASSWORD= \
  # APP_HOST -> Listen address: 0.0.0.0 means listen only on IPv4; :: means listen on both IPv4 and IPv6
  -e APP_HOST=0.0.0.0 \
  # VITE_API_URL -> Access address: Server/NAS address, e.g., 192.168.1.100 or your domain
  -e VITE_API_URL= \
  # VITE_API_PORT -> Running port: e.g., 3000 (should match the mapped port in bridge mode)
  -e VITE_API_PORT= \
  # JWT_SECRET -> TOKEN encryption key: a string used to encrypt the token
  -e JWT_SECRET= \
  # JWT_EXPIRE -> TOKEN expiration time: default 1200 seconds (20 minutes)
  -e JWT_EXPIRE=1200 \
  nas-tiktok:latest
```

#### Additional Notes

1. This project has features for logging out on IP change and LAN-only access. It is recommended to run it in Docker's --network host mode, as this mode can retrieve the client's real IP; otherwise, these two features will be ineffective.

2. SSL: If you have a domain, it's strongly recommended to enable it! This will encrypt your data traffic and protect your privacy. If you don't have a domain, it's advisable to limit access to LAN only.

3. LAN_ONLY: Only works in an IPv4 environment and when --network host is used, because Docker can only get the real access IP when bound to the host, otherwise it cannot impose restrictions.

4. APP_HOST: If set to ::, IPv6 is enabled. If you have an IPv6 environment, you need to disable LAN_ONLY, as IPv6 cannot determine if access is from the local network and will block all connections.

## Features and Suggestions

If you have any features or suggestions, please feel free to raise them in the `Issues`

If you are willing to contribute to this project, pull requests are welcome! Thank you very much for your support!

## Contact Me

You can contact me via email: <a href="mailto:ASentimentalMan@GMail.com">ASentimentalMan@GMail.com</a>

## License

[GPLv3](LICENSE)
