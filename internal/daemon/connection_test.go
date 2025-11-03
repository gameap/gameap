package daemon

var daemonServerCert = `-----BEGIN CERTIFICATE-----
MIIDMDCCAhigAwIBAgIQG5521n28JMsvl5EbNVh74jANBgkqhkiG9w0BAQsFADAy
MQswCQYDVQQGEwJSVTEPMA0GA1UECgwGR2FtZUFQMRIwEAYDVQQDDAlHYW1lQVAg
Q0EwHhcNMjUxMDEzMTQxODE0WhcNMzUxMDEzMTQxODE0WjAyMQswCQYDVQQGEwJS
VTEPMA0GA1UECgwGR2FtZUFQMRIwEAYDVQQDDAlHYW1lQVAgQ0EwggEiMA0GCSqG
SIb3DQEBAQUAA4IBDwAwggEKAoIBAQDhv9T03BFuSzzszJZK7tXrcCVhlJrw0/U/
xf9DpevR7OAZODxRvT3CUPhCcs8zsOec1pxGRe9iYiKm557Pu9fcxhU9IBKSIXfN
3T6fo6b7chEsuA+gzgvToKUh08ilefK+RPVlZRYggSlq8nqLmRxKzDBspyBgutkN
/2PpWE21cqBw0E2lBDbCE18oOasS5rKy2rsrrxtS2Ne6h3hh4Wfk1dqs9jrFNbx6
toUNZhMgFZ7Jp4XrvTwRQJl9JPSUWRNDr+MytlTu3d+RkRPHxGul2uWSYyIvbykf
GWhZVLsB3Omq9+K8tCQFJh5aEi95CnPvO0360Tn2TvDLU20GXZddAgMBAAGjQjBA
MA8GA1UdEwEB/wQFMAMBAf8wHQYDVR0OBBYEFOXIoN9uq0tcZ1NJHUcmmxB+g460
MA4GA1UdDwEB/wQEAwIChDANBgkqhkiG9w0BAQsFAAOCAQEAZG7RHTarUgsLn0XX
vlRadkf/A7Kb9iZfQWJH1koA95k3URXXSUzk060CH32OU8i19bS26OkIJuM7a7sA
4IHinOr3OUoC6X3YNJCGNb5UroCT11Xg0+7cIgtJpyZR9PuGd3Vm+CT/tRgYJ9LZ
EmWM61te0jpqnhVyqwtfEcL5rU/+lr/LOAIxHNbjHa/CqArs6hV04NuY7h/yaX3u
FhgRjKghmHWq9RtqlIb1E4RvsYGjq7aGyDU2TzZ4KnfYFjonzD8hLVGLHW8n0grq
k2dV/yjA4+iTtQ5Q7j8oWdzNau+sNyBhdFaQo9wNCbcOtY3AiAXH6DwQ66vSnah3
NNMoIQ==
-----END CERTIFICATE-----`

var daemonServerKey = `-----BEGIN PRIVATE KEY-----
MIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQDhv9T03BFuSzzs
zJZK7tXrcCVhlJrw0/U/xf9DpevR7OAZODxRvT3CUPhCcs8zsOec1pxGRe9iYiKm
557Pu9fcxhU9IBKSIXfN3T6fo6b7chEsuA+gzgvToKUh08ilefK+RPVlZRYggSlq
8nqLmRxKzDBspyBgutkN/2PpWE21cqBw0E2lBDbCE18oOasS5rKy2rsrrxtS2Ne6
h3hh4Wfk1dqs9jrFNbx6toUNZhMgFZ7Jp4XrvTwRQJl9JPSUWRNDr+MytlTu3d+R
kRPHxGul2uWSYyIvbykfGWhZVLsB3Omq9+K8tCQFJh5aEi95CnPvO0360Tn2TvDL
U20GXZddAgMBAAECggEAX7v64wYqjDPy99cBC3ECnhAWgi3DkUrJ0Aw25ujHu2Qx
WvCav+05chzlU0Sd8yVb64qlhjWlQXsth8tk8WKPNex42E2wInF3/YEMTCXaK/rh
Jq49zmti35HaRaCrD+XJ1/+lc6TtP8aWmmiPKIE7WssB5Cnx1KOYZdO++pd8iOvx
9VzPWQCUDuAwrmwJ+ObuAn02ZgL2shzjlIxfJKf6DVYq7JYZYOh873buPZjUUqMy
VG89LuG0zyVXRVqJ0Ro6K3JIsO4Piqm4CjZxulyiXmSIvezvq8Zka/awEiC7KaOy
Xq4baB/pPdroZCQXx8a+8z8icUmlvZY4kpStSQmabwKBgQD4bu11X/emTvfO2Vpi
Q2surTwJU73FyrwsfB9pQJZwLGE2oYQOwfTkm0n4lQVmumqhYcnmJeGhmYMfVFEN
0R6FadGRfA6P74pwsuZdRTLgWiDoRFrQIbiaP8VRnbFJDfZTLRK5utZ/JJyzAxwv
FpMDKP7C+ZRtt2Y7ny9Xg8/45wKBgQDooAivzphDnZLAHN4caKjgbbsq1FoFatMP
pBRJbU1IRzBoM8y3sKYzAN3AX+ct9NhqMBUiQtXV4XysUknq0/lg2mcx9MlgZWWC
DqN0gnIJDhF0/pSSnfCyZXk8pvh02v2VVRJajm3b8X0NFPV1cgFnDoPumJ3lhgO8
e7fWEQERGwKBgQDJM2WjWu7BxVDTOJsH3BwxOGHYF/co+nF+AaSa5JEyFe9BhHvk
S9cfUlkNNvuh4DY9r2oJuAJNk3trYykl7IgweqwcjIFqtxDDB1Ckl0eGBdiC4+E8
kSLl4eHXoMQVK3aklGuG+jd/z1INdOZdiIXV2FzD4cgBN7hXbyuzT/CeXwKBgQDd
8dJX6oTb/jtFkEVaVYDKn+cztectw/4brjCs3dweWc2VndZ0a9YmU20/XkDzV+gj
aDzBs4LRzZFl2B0uL5B/F+Hdh++aSSPQMqdBQjQK76E4Pq0CNi6wBqDlfWkQFOBc
2g3o2Ht4na7yDE0lAODVssOtoW8EzhhCfMepNhAOgQKBgHZEC8DMMn3/2Pz35yfy
4v+4keau2jQlOqODZBV9xjSddDT+fMqnBQQ7uqgT3npf4o4ckKm4f8U3z/PY0gM/
F0EPpQEiHyLmZ560z3CpPJHdKWj4ulKzG7epai8LNnutvxQVjoNMCZ6su2xITa9h
kYl8ehAXJrIFnRZAldZxgk/8
-----END PRIVATE KEY-----`

var clientCert = `-----BEGIN CERTIFICATE-----
MIIDBzCCAe8CEAinETuffugXS+3ZyVqHalowDQYJKoZIhvcNAQELBQAwMjELMAkG
A1UEBhMCUlUxDzANBgNVBAoMBkdhbWVBUDESMBAGA1UEAwwJR2FtZUFQIENBMB4X
DTI1MTAxMzE0MTgxNFoXDTM1MTAxMzE0MTgxNFowLTEPMA0GA1UECgwGR2FtZUFQ
MRowGAYDVQQDDBF1YnVudHUtMmdiLW5iZzEtMTCCASIwDQYJKoZIhvcNAQEBBQAD
ggEPADCCAQoCggEBAMVLe+7XTqDWBU3YcIp99RveJsTXrXNSebpQ/8ww4yIbuciW
qG3hiJepnkHkyNytdYTCIYQm5SdJjVMn+nGThjvrPKOZw05fRdjrC6L89ulE1VLv
IFhU+2Ojsydwk/To54QRBidNMUieS2DqCaK3FzVqNP8egGE5sXy9QfVcnqFemViV
OmLIH6ll+WHFbux/TwYiO0EbgXTkQTBRuIQJNsFBsX8O8qXbqTBQh+as8f53jT2K
B1Q6yictFXnhGG+FxckcX6nXy1NzJaMpNtnvqwj5HJFWkXgazuyebPe1ycYMJyPb
YRZgNU/mNxNLiPYBTj4uQvPI709KO3XqYjOjtv8CAwEAAaMjMCEwHwYDVR0jBBgw
FoAU5cig326rS1xnU0kdRyabEH6DjrQwDQYJKoZIhvcNAQELBQADggEBADWL6G+l
KO0cmW6lX8hxgHuXZySXmzk4DjUjisuQZ5yltp4LUYWcWWot2yHdaCBbu7rDaYqV
VeFNdtjLtCSUo1FlnJ3LQCFoTiqxyOtSY7/oMEUK7jksplsYsT5dTrV9NAtVFXCu
TZpm91bw/4B8swYt4xyxf5PZGCKGuSbZRAyIGM1CBghbUcQOFnJp/72R7aU4ckel
zQGfr3QhIdLkiM2sfMgjUOOXgmm7C4Hp84UkY3ymOgKh5rCr/NntgpJW0rVzyjcl
VJOqf3WvC3acT5MFiVWB+hS+Dt6pSeQ5GcBob/0BFOIlKrR1ApfPbiZnsRQS/Hoy
/ryz/+SJsuFXzqU=
-----END CERTIFICATE-----`

var clientKey = `-----BEGIN PRIVATE KEY-----
MIIEvAIBADANBgkqhkiG9w0BAQEFAASCBKYwggSiAgEAAoIBAQDFS3vu106g1gVN
2HCKffUb3ibE161zUnm6UP/MMOMiG7nIlqht4YiXqZ5B5MjcrXWEwiGEJuUnSY1T
J/pxk4Y76zyjmcNOX0XY6wui/PbpRNVS7yBYVPtjo7MncJP06OeEEQYnTTFInktg
6gmitxc1ajT/HoBhObF8vUH1XJ6hXplYlTpiyB+pZflhxW7sf08GIjtBG4F05EEw
UbiECTbBQbF/DvKl26kwUIfmrPH+d409igdUOsonLRV54RhvhcXJHF+p18tTcyWj
KTbZ76sI+RyRVpF4Gs7snmz3tcnGDCcj22EWYDVP5jcTS4j2AU4+LkLzyO9PSjt1
6mIzo7b/AgMBAAECggEATJXgpfYuL4DkzjMWfKwoWYkCw6Z1Ti7V0d1fboQLp1Hb
7GGPQBgsTbMqG6oTzpYG6GHzYLk4eueyVHVQYoZBtUC7aUZm6iVRl8Kl4b8Qmbx+
kpMAm0lhzGvfP5AT3x4JwNpa6Sat2uKXoCc5VlB8Ud/IcsAEVblvjFxrHjO14C2K
HXoPwVa40gpaYcIFvH2x6f7IyENNVShUgdp1ibIaiTSzgDGWrS43TTohPyId+1LN
pFKk5k/6B2SA1FTLGrfT+bAGY5S9VSE1sX1ZEij4Fe4OYID3naR5/UOzpx9trLJD
cCTkU9Z84UhIzHToElnPkdhVFWwklvD92Ir+yYe2AQKBgQDiMTG5RUl4XQprPTaI
vKFgKQBLBTohNiD2Wyk1YLGaTrJVw1TB2+LfUNgr9vaFupzG81xbb4kT21rSVNDr
wY1CnL2qQsvZXD+HfBxHRJSYEcd+GINBzI4JTYXF0g1UPfub0Ui6cf9SuIphgiJc
7CHI7xKNahQezNqU5GmRkseEGQKBgQDfS2l9TB8TFZVkezc8gFXmF5IhwL/9yQKh
rG8eoNmpYA1MY3HUYjwLSY8TjSSUitm343QZctBMw02Cp0K8do4Zsu5TUDuYBlnU
D/77qfGsQTHlz++6RTBwRUcsrGOpQOaFLhoDIgP6LlSmBcCiJOc6zouzJTndrlWu
M2L0fLq21wKBgH1YwbNoICTheoqfK39u+Qbu8cihJuuMsYuUTSvVX9ahUdaRHoEn
t3wFsyX5//dvyL2/0yigkJg+cQAqHHTpl7yYW8rkpU7Y/iO4tXsRGD+FasYIE4T9
NKEXItDTbKuIhcx9mA4qalGPDrCmiyBvgvF0+xT++hNvdpoYUiBn9MTRAoGAefSk
LuzuY+v75h9t8bteLwdcptaxhZjNuSOGpUHQ37M4UCpYN1lX1gpc/J6wBfk4JDk0
ZdnRbruUj/Fuf6R4xAx4IkTF56hAU5RQ/X66IgyRhiTll+TGKeuMjhexbvWlccPW
LTPc3D2Fug+WQHjLWdEJd9SzICJhZX1nZITjLY0CgYA0yT0Aq8c4ZYubnDRoDygW
Ek8Ha640AMI481GzkiOVbz3RQgg5Iu6nsCtRBl1BZtYXZDqAf4s4MaDZq2lkWhL3
Gh4tgcWeZpJsPDp6XhQOd350PUMKMDe7OpFdAoRB3uTPlf3HcBp0YyZoX6OfqPe4
C34XjQ9lMiVDZ8YpDfK0/Q==
-----END PRIVATE KEY-----`

// func Test_List(t *testing.T) {
//	pool, err := NewPool(config{
//		Host: "127.0.0.1",
//		Port: 31717,
//		// Username: "gameap",
//		// Password: "gameap123",
//		ServerCertificate: []byte(daemonServerCert),
//		ClientCertificate: []byte(clientCert),
//		PrivateKey:        []byte(clientKey),
//		Timeout:           10 * time.Second,
//		Mode:              binnapi.ModeFiles,
//	})
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	for i := 0; i < 5; i++ {
//		func() {
//			conn, err := pool.Acquire(context.Background())
//			require.NoError(t, err)
//			defer func(conn net.Conn) {
//				err := conn.Close()
//				if err != nil {
//					t.Fatal(err)
//				}
//			}(conn)
//
//			err = binnapi.WriteMessage(conn, &binnapi.FileInfoRequestMessage{
//				Path: "/srv/gameap",
//			})
//			require.NoError(t, err)
//
//			var resp binnapi.BaseResponseMessage
//			err = decode.NewDecoder(conn).Decode(&resp)
//			require.NoError(t, err)
//
//			fi, err := binnapi.CreateFileDetailsResponseMessage(resp.Data)
//			require.NoError(t, err)
//
//			_ = fi
//		}()
//
//		time.Sleep(10 * time.Second)
//	}
//}
//
//
// func Test_Download(t *testing.T) {
//	pool, err := NewPool(config{
//		Host: "127.0.0.1",
//		Port: 31717,
//		// Username: "gameap",
//		// Password: "gameap123",
//		ServerCertificate: []byte(daemonServerCert),
//		ClientCertificate: []byte(clientCert),
//		PrivateKey:        []byte(clientKey),
//		Timeout:           10 * time.Second,
//		Mode:              binnapi.ModeFiles,
//	})
//	if err != nil {
//		t.Fatal(err)
//	}
//
//	conn, err := pool.Acquire(context.Background())
//	require.NoError(t, err)
//	defer func(conn net.Conn) {
//		err := conn.Close()
//		if err != nil {
//			t.Fatal(err)
//		}
//	}(conn)
//
//	err = binnapi.WriteMessage(conn, &binnapi.DownloadRequestMessage{
//		FilePath: "/root/file.txt",
//	})
//
//	var resp binnapi.BaseResponseMessage
//	err = decode.NewDecoder(conn).Decode(&resp)
//	require.NoError(t, err)
//
//	err = binnapi.ReadEndBytes(context.Background(), conn)
//	require.NoError(t, err)
//
//	if resp.Code != binnapi.StatusCodeReadyToTransfer {
//		t.Fatalf("expected status code %d, got %d", binnapi.StatusCodeReadyToTransfer, resp.Code)
//	}
//
//	fileSize, err := binnapi.CreateFileSize(resp.Data)
//	require.NoError(t, err)
//	require.NotZero(t, fileSize)
//
//	file := make([]byte, fileSize)
//
//	n, err := conn.Read(file)
//	require.NoError(t, err)
//	require.Equal(t, int(fileSize), n)
//}
