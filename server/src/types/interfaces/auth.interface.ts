declare interface JWT {
  host: string;
  connection: string;
  'user-agent': string;
  dnt: string;
  accept: string;
  'sec-fetch-site': string;
  referer: string;
  'accept-language': string;
  session: string;
  ip: string;
  iat?: number;
  exp?: number;
}
