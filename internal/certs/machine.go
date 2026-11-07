package certs

import (
	"bytes"
	"encoding/pem"
	"errors"
	"kiltro/internal/i18n"
	"os"
	"path/filepath"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"kiltro/internal/procutil"
)

// Almacén de certificados de la MÁQUINA ("Equipo local → Entidades de
// certificación raíz de confianza").
//
// `mkcert -install` instala la CA en el almacén del USUARIO: no necesita admin,
// pero Windows muestra un aviso de seguridad que asusta. Si la CA está en el
// almacén de la máquina, Windows (y Chrome/Edge) ya confían en ella y no hace
// falta ese paso. Escribir ahí requiere admin, así que lo hace el instalador
// (kiltro.exe --install-ca). Leerlo NO requiere admin: Kiltro lo revisa antes de
// activar HTTPS para saltarse el aviso.

// caFile es el certificado público de la CA de Kiltro (rootCA.pem).
func caFile() string { return filepath.Join(CARoot(), "rootCA.pem") }

// CreateCA crea la CA de Kiltro si no existe. mkcert solo crea la CA cuando
// firma algo, así que se le pide el certificado de localhost (que Nginx usa
// igual para el servidor por defecto del 443).
func CreateCA(job *procutil.Job) error {
	if _, err := os.Stat(caFile()); err == nil {
		return nil
	}
	if err := os.MkdirAll(CARoot(), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(certDir(), 0o755); err != nil {
		return err
	}
	exe, err := Exe()
	if err != nil {
		return err
	}
	cert, key := Files("localhost")
	if out, err := run(job, exe, false, 30*time.Second, "-cert-file", cert, "-key-file", key, "localhost"); err != nil {
		return i18n.Err("certs.caFailed", "out", out)
	}
	return nil
}

// readCA devuelve el certificado de la CA en formato DER (el binario que
// guarda Windows), leyendo el .pem indicado.
func readCA(pemFile string) ([]byte, error) {
	data, err := os.ReadFile(pemFile)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, i18n.Err("certs.notPem", "path", pemFile)
	}
	return block.Bytes, nil
}

// openRoot abre el almacén "Root" (entidades raíz de confianza) de la máquina
// (CERT_SYSTEM_STORE_LOCAL_MACHINE) o del usuario (CERT_SYSTEM_STORE_CURRENT_USER).
// readOnly=true no necesita admin.
func openRoot(location uint32, readOnly bool) (windows.Handle, error) {
	flags := location | windows.CERT_STORE_OPEN_EXISTING_FLAG
	if readOnly {
		flags |= windows.CERT_STORE_READONLY_FLAG
	}
	name, _ := windows.UTF16PtrFromString("Root")
	return windows.CertOpenStore(windows.CERT_STORE_PROV_SYSTEM, 0, 0, flags, uintptr(unsafe.Pointer(name)))
}

// findIn busca el certificado (DER) en el almacén. Devuelve el contexto (hay
// que liberarlo con CertFreeCertificateContext) o nil si no está.
func findIn(store windows.Handle, der []byte) *windows.CertContext {
	var ctx *windows.CertContext
	for {
		next, err := windows.CertEnumCertificatesInStore(store, ctx) // libera el anterior solo
		if err != nil || next == nil {
			return nil
		}
		ctx = next
		encoded := unsafe.Slice(ctx.EncodedCert, ctx.Length)
		if bytes.Equal(encoded, der) {
			return windows.CertDuplicateCertificateContext(ctx)
		}
	}
}

// TrustedByMachine dice si la CA de Kiltro ya está en el almacén de la
// máquina (false si todavía no hay CA).
func TrustedByMachine() bool { return trustedIn(windows.CERT_SYSTEM_STORE_LOCAL_MACHINE) }

// TrustedByUser dice si la CA está en el almacén del usuario (lo que hace
// `mkcert -install`, con el aviso de Windows). Ojo: el almacén "Root" del
// usuario INCLUYE los de la máquina, así que esto también da true si la CA está
// solo en la máquina. Para saber si está solo en el del usuario, preguntar
// después de quitarla de la máquina.
func TrustedByUser() bool { return trustedIn(windows.CERT_SYSTEM_STORE_CURRENT_USER) }

func trustedIn(location uint32) bool {
	der, err := readCA(caFile())
	if err != nil {
		return false
	}
	store, err := openRoot(location, true)
	if err != nil {
		return false
	}
	defer windows.CertCloseStore(store, 0)
	if ctx := findIn(store, der); ctx != nil {
		windows.CertFreeCertificateContext(ctx)
		return true
	}
	return false
}

// AddToMachine agrega la CA al almacén de la máquina. REQUIERE ADMIN (lo llama
// el instalador con kiltro.exe --install-ca). No muestra ningún aviso.
func AddToMachine() error {
	der, err := readCA(caFile())
	if err != nil {
		return err
	}
	ctx, err := windows.CertCreateCertificateContext(windows.X509_ASN_ENCODING|windows.PKCS_7_ASN_ENCODING, &der[0], uint32(len(der)))
	if err != nil {
		return i18n.Wrap(err, "certs.invalid")
	}
	defer windows.CertFreeCertificateContext(ctx)

	store, err := openRoot(windows.CERT_SYSTEM_STORE_LOCAL_MACHINE, false)
	if err != nil {
		return i18n.Wrap(err, "certs.storeOpen")
	}
	defer windows.CertCloseStore(store, 0)
	if err := windows.CertAddCertificateContextToStore(store, ctx, windows.CERT_STORE_ADD_REPLACE_EXISTING, nil); err != nil {
		return i18n.Wrap(err, "certs.storeAdd")
	}
	return nil
}

// RemoveFromMachine quita del almacén de la máquina el certificado del .pem
// indicado. REQUIERE ADMIN. Si no estaba, no hace nada.
func RemoveFromMachine(pemFile string) error {
	der, err := readCA(pemFile)
	if errors.Is(err, os.ErrNotExist) {
		return nil // sin CA no hay nada que quitar
	}
	if err != nil {
		return err
	}
	store, err := openRoot(windows.CERT_SYSTEM_STORE_LOCAL_MACHINE, false)
	if err != nil {
		return i18n.Wrap(err, "certs.storeOpen")
	}
	defer windows.CertCloseStore(store, 0)
	ctx := findIn(store, der)
	if ctx == nil {
		return nil
	}
	// CertDeleteCertificateFromStore libera el contexto que recibe.
	if err := windows.CertDeleteCertificateFromStore(ctx); err != nil {
		return i18n.Wrap(err, "certs.storeRemove")
	}
	return nil
}

// CAFile es la ruta de rootCA.pem (para pasársela al proceso elevado).
func CAFile() string { return caFile() }
