package main

import (
	"os"
	"strings"
)

// Every string a person reads, in each language ZipClick speaks. The language
// is the first of the user's Windows display languages that is in the catalog
// (ZIPCLICK_LANG goes first, for testing), else English.
//
// Strings that name Windows' own menus ("Show more options", "Extract All…")
// must match what that Windows says, so check a translation against the real
// menu, not a dictionary.

type messages struct {
	InstallPrompt, Installed, AlmostDone, Removed string

	CouldNotExtract string // %s: the archive's name
	Encrypted       string
	Empty           string
	Damaged         string
	Method          string // %d: the compression method number
	TryExtractAll   string

	Extracting string // %s: the archive's name, quoted the way the language quotes
	Cancel     string
	Stopping   string
	Of         string // "%s of %s": done, total
	FewSeconds string
	Seconds    string // %d
	Minute     string
	Minutes    string // %d
	Bytes      string
	Units      [4]string // KB, MB, GB, TB
	Decimal    string

	// Written to the registry at install time.
	VerbExtract, VerbExtractHere, TypeName, AppDescription string
}

var catalog = map[string]*messages{
	"en":    &en,
	"pt-PT": &ptPT,
	"pt-BR": &ptBR,
	"es":    &es,
	"fr":    &fr,
	"de":    &de,
	"it":    &it,
}

// msg is the language in use; main sets it before anything is shown.
var msg = &en

func setLanguage() {
	prefs := preferredLanguages()
	if v := os.Getenv("ZIPCLICK_LANG"); v != "" {
		prefs = append([]string{v}, prefs...)
	}
	msg = catalog[pickLanguage(prefs)]
}

// pickLanguage maps preferences like "pt-BR", "de-AT" or "fr" to a catalog
// key: an exact match first, then the base language. Portuguese other than
// Brazil's reads best as Portugal's.
func pickLanguage(prefs []string) string {
	for _, p := range prefs {
		p = strings.ReplaceAll(p, "_", "-")
		for k := range catalog {
			if strings.EqualFold(p, k) {
				return k
			}
		}
		base, _, _ := strings.Cut(strings.ToLower(p), "-")
		if base == "pt" {
			return "pt-PT"
		}
		if _, ok := catalog[base]; ok {
			return base
		}
	}
	return "en"
}

var en = messages{
	InstallPrompt: "Make double-clicking a .zip file extract it, like on a Mac?\n\n" +
		"ZipClick installs for your user only. No administrator rights needed.",
	Installed: "ZipClick is installed and already the default for .zip files.",
	AlmostDone: "Almost done. Windows only lets you choose the default app yourself.\n\n" +
		"In the Settings page that opens next, click the box under .zip and choose ZipClick.\n\n" +
		"Until then you can also right-click any .zip > Show more options > Extract here (ZipClick).",
	Removed: "ZipClick has been removed. Double-clicking a .zip opens it in Explorer again.",

	CouldNotExtract: "Could not extract %s.",
	Encrypted:       "The archive is password-protected.",
	Empty:           "The archive is empty.",
	Damaged:         "This isn't a valid zip file, or it is damaged.",
	Method:          "It uses a compression method ZipClick doesn't support (%d).",
	TryExtractAll:   "Right-click the file > Extract All… to use Windows' own extractor instead.",

	Extracting: "Extracting “%s”",
	Cancel:     "Cancel",
	Stopping:   "Stopping…",
	Of:         "%s of %s",
	FewSeconds: "a few seconds left",
	Seconds:    "about %d seconds left",
	Minute:     "about a minute left",
	Minutes:    "about %d minutes left",
	Bytes:      "bytes",
	Units:      [4]string{"KB", "MB", "GB", "TB"},
	Decimal:    ".",

	VerbExtract:     "Extract",
	VerbExtractHere: "Extract here (ZipClick)",
	TypeName:        "ZIP archive",
	AppDescription:  "Extract .zip files with a double-click, like on a Mac.",
}

var ptPT = messages{
	InstallPrompt: "Fazer com que um duplo clique num ficheiro .zip o extraia, como num Mac?\n\n" +
		"O ZipClick é instalado só para o seu utilizador. Não precisa de permissões de administrador.",
	Installed: "O ZipClick está instalado e já é a aplicação predefinida para ficheiros .zip.",
	AlmostDone: "Quase pronto. O Windows só deixa que seja o próprio utilizador a escolher a aplicação predefinida.\n\n" +
		"Na página das Definições que vai abrir, clique na caixa por baixo de .zip e escolha ZipClick.\n\n" +
		"Até lá, também pode clicar com o botão direito em qualquer .zip > Mostrar mais opções > Extrair aqui (ZipClick).",
	Removed: "O ZipClick foi removido. Um duplo clique num .zip volta a abri-lo no Explorador de Ficheiros.",

	CouldNotExtract: "Não foi possível extrair %s.",
	Encrypted:       "O arquivo está protegido por palavra-passe.",
	Empty:           "O arquivo está vazio.",
	Damaged:         "Não é um ficheiro zip válido, ou está danificado.",
	Method:          "Usa um método de compressão que o ZipClick não suporta (%d).",
	TryExtractAll:   "Clique com o botão direito no ficheiro > Extrair Tudo… para usar o extrator do próprio Windows.",

	Extracting: "A extrair «%s»",
	Cancel:     "Cancelar",
	Stopping:   "A parar…",
	Of:         "%s de %s",
	FewSeconds: "faltam poucos segundos",
	Seconds:    "faltam cerca de %d segundos",
	Minute:     "falta cerca de um minuto",
	Minutes:    "faltam cerca de %d minutos",
	Bytes:      "bytes",
	Units:      [4]string{"KB", "MB", "GB", "TB"},
	Decimal:    ",",

	VerbExtract:     "Extrair",
	VerbExtractHere: "Extrair aqui (ZipClick)",
	TypeName:        "Arquivo ZIP",
	AppDescription:  "Extraia ficheiros .zip com um duplo clique, como num Mac.",
}

var ptBR = messages{
	InstallPrompt: "Fazer com que um clique duplo em um arquivo .zip o extraia, como no Mac?\n\n" +
		"O ZipClick é instalado só para o seu usuário. Não precisa de permissão de administrador.",
	Installed: "O ZipClick está instalado e já é o aplicativo padrão para arquivos .zip.",
	AlmostDone: "Quase pronto. O Windows só deixa você mesmo escolher o aplicativo padrão.\n\n" +
		"Na página de Configurações que vai abrir, clique na caixa abaixo de .zip e escolha ZipClick.\n\n" +
		"Até lá, você também pode clicar com o botão direito em qualquer .zip > Mostrar mais opções > Extrair aqui (ZipClick).",
	Removed: "O ZipClick foi removido. Um clique duplo em um .zip volta a abri-lo no Explorador de Arquivos.",

	CouldNotExtract: "Não foi possível extrair %s.",
	Encrypted:       "O arquivo está protegido por senha.",
	Empty:           "O arquivo está vazio.",
	Damaged:         "Não é um arquivo zip válido, ou está corrompido.",
	Method:          "Ele usa um método de compactação que o ZipClick não suporta (%d).",
	TryExtractAll:   "Clique com o botão direito no arquivo > Extrair Tudo… para usar o extrator do próprio Windows.",

	Extracting: "Extraindo “%s”",
	Cancel:     "Cancelar",
	Stopping:   "Parando…",
	Of:         "%s de %s",
	FewSeconds: "faltam alguns segundos",
	Seconds:    "faltam cerca de %d segundos",
	Minute:     "falta cerca de um minuto",
	Minutes:    "faltam cerca de %d minutos",
	Bytes:      "bytes",
	Units:      [4]string{"KB", "MB", "GB", "TB"},
	Decimal:    ",",

	VerbExtract:     "Extrair",
	VerbExtractHere: "Extrair aqui (ZipClick)",
	TypeName:        "Arquivo ZIP",
	AppDescription:  "Extraia arquivos .zip com um clique duplo, como no Mac.",
}

var es = messages{
	InstallPrompt: "¿Quieres que al hacer doble clic en un archivo .zip se extraiga, como en un Mac?\n\n" +
		"ZipClick se instala solo para tu usuario. No hacen falta permisos de administrador.",
	Installed: "ZipClick está instalado y ya es la aplicación predeterminada para los archivos .zip.",
	AlmostDone: "Casi listo. Windows solo te deja elegir a ti la aplicación predeterminada.\n\n" +
		"En la página de Configuración que se abrirá, haz clic en el cuadro bajo .zip y elige ZipClick.\n\n" +
		"Mientras tanto, también puedes hacer clic con el botón derecho en cualquier .zip > Mostrar más opciones > Extraer aquí (ZipClick).",
	Removed: "ZipClick se ha eliminado. Al hacer doble clic en un .zip se vuelve a abrir en el Explorador de archivos.",

	CouldNotExtract: "No se ha podido extraer %s.",
	Encrypted:       "El archivo está protegido con contraseña.",
	Empty:           "El archivo está vacío.",
	Damaged:         "No es un archivo zip válido, o está dañado.",
	Method:          "Usa un método de compresión que ZipClick no admite (%d).",
	TryExtractAll:   "Haz clic con el botón derecho en el archivo > Extraer todo… para usar el extractor del propio Windows.",

	Extracting: "Extrayendo «%s»",
	Cancel:     "Cancelar",
	Stopping:   "Deteniendo…",
	Of:         "%s de %s",
	FewSeconds: "quedan unos segundos",
	Seconds:    "quedan unos %d segundos",
	Minute:     "queda alrededor de un minuto",
	Minutes:    "quedan unos %d minutos",
	Bytes:      "bytes",
	Units:      [4]string{"KB", "MB", "GB", "TB"},
	Decimal:    ",",

	VerbExtract:     "Extraer",
	VerbExtractHere: "Extraer aquí (ZipClick)",
	TypeName:        "Archivo ZIP",
	AppDescription:  "Extrae archivos .zip con un doble clic, como en un Mac.",
}

// French puts a no-break space inside « » and before ? and :.
var fr = messages{
	InstallPrompt: "Extraire les fichiers .zip d’un double-clic, comme sur un Mac ?\n\n" +
		"ZipClick s’installe uniquement pour votre compte, sans droits d’administrateur.",
	Installed: "ZipClick est installé et c’est déjà l’application par défaut pour les fichiers .zip.",
	AlmostDone: "Presque terminé. Windows ne laisse que vous choisir l’application par défaut.\n\n" +
		"Dans la page Paramètres qui va s’ouvrir, cliquez sur la case sous .zip et choisissez ZipClick.\n\n" +
		"En attendant, vous pouvez aussi faire un clic droit sur un .zip > Afficher d’autres options > Extraire ici (ZipClick).",
	Removed: "ZipClick a été supprimé. Un double-clic sur un .zip l’ouvre à nouveau dans l’Explorateur de fichiers.",

	CouldNotExtract: "Impossible d’extraire %s.",
	Encrypted:       "L’archive est protégée par un mot de passe.",
	Empty:           "L’archive est vide.",
	Damaged:         "Ce n’est pas un fichier zip valide, ou il est endommagé.",
	Method:          "Elle utilise une méthode de compression que ZipClick ne prend pas en charge (%d).",
	TryExtractAll:   "Faites un clic droit sur le fichier > Extraire tout… pour utiliser l’extracteur de Windows.",

	Extracting: "Extraction de « %s »",
	Cancel:     "Annuler",
	Stopping:   "Arrêt…",
	Of:         "%s sur %s",
	FewSeconds: "encore quelques secondes",
	Seconds:    "encore environ %d secondes",
	Minute:     "encore environ une minute",
	Minutes:    "encore environ %d minutes",
	Bytes:      "octets",
	Units:      [4]string{"Ko", "Mo", "Go", "To"},
	Decimal:    ",",

	VerbExtract:     "Extraire",
	VerbExtractHere: "Extraire ici (ZipClick)",
	TypeName:        "Archive ZIP",
	AppDescription:  "Extrayez les fichiers .zip d’un double-clic, comme sur un Mac.",
}

var de = messages{
	InstallPrompt: "Sollen .zip-Dateien per Doppelklick entpackt werden, wie auf einem Mac?\n\n" +
		"ZipClick wird nur für Ihr Benutzerkonto installiert. Administratorrechte sind nicht nötig.",
	Installed: "ZipClick ist installiert und bereits die Standard-App für .zip-Dateien.",
	AlmostDone: "Fast fertig. Die Standard-App können unter Windows nur Sie selbst festlegen.\n\n" +
		"Klicken Sie auf der Einstellungsseite, die sich gleich öffnet, auf das Feld unter .zip und wählen Sie ZipClick.\n\n" +
		"Bis dahin können Sie auch mit der rechten Maustaste auf eine .zip-Datei klicken > Weitere Optionen anzeigen > Hier entpacken (ZipClick).",
	Removed: "ZipClick wurde entfernt. Ein Doppelklick auf eine .zip-Datei öffnet sie wieder im Datei-Explorer.",

	CouldNotExtract: "%s konnte nicht entpackt werden.",
	Encrypted:       "Das Archiv ist kennwortgeschützt.",
	Empty:           "Das Archiv ist leer.",
	Damaged:         "Das ist keine gültige ZIP-Datei, oder sie ist beschädigt.",
	Method:          "Sie verwendet eine Komprimierungsmethode, die ZipClick nicht unterstützt (%d).",
	TryExtractAll:   "Klicken Sie mit der rechten Maustaste auf die Datei > Alle extrahieren…, um das Entpacken von Windows zu verwenden.",

	Extracting: "„%s“ wird entpackt",
	Cancel:     "Abbrechen",
	Stopping:   "Wird angehalten…",
	Of:         "%s von %s",
	FewSeconds: "noch wenige Sekunden",
	Seconds:    "noch etwa %d Sekunden",
	Minute:     "noch etwa eine Minute",
	Minutes:    "noch etwa %d Minuten",
	Bytes:      "Byte",
	Units:      [4]string{"KB", "MB", "GB", "TB"},
	Decimal:    ",",

	VerbExtract:     "Entpacken",
	VerbExtractHere: "Hier entpacken (ZipClick)",
	TypeName:        "ZIP-Archiv",
	AppDescription:  ".zip-Dateien per Doppelklick entpacken, wie auf einem Mac.",
}

var it = messages{
	InstallPrompt: "Vuoi che un doppio clic su un file .zip lo estragga, come su un Mac?\n\n" +
		"ZipClick si installa solo per il tuo utente: non servono diritti di amministratore.",
	Installed: "ZipClick è installato ed è già l’app predefinita per i file .zip.",
	AlmostDone: "Quasi fatto. Windows permette solo a te di scegliere l’app predefinita.\n\n" +
		"Nella pagina Impostazioni che si aprirà, fai clic sulla casella sotto .zip e scegli ZipClick.\n\n" +
		"Nel frattempo puoi anche fare clic con il tasto destro su un .zip > Mostra altre opzioni > Estrai qui (ZipClick).",
	Removed: "ZipClick è stato rimosso. Un doppio clic su un .zip lo apre di nuovo in Esplora file.",

	CouldNotExtract: "Impossibile estrarre %s.",
	Encrypted:       "L’archivio è protetto da password.",
	Empty:           "L’archivio è vuoto.",
	Damaged:         "Non è un file zip valido, oppure è danneggiato.",
	Method:          "Usa un metodo di compressione che ZipClick non supporta (%d).",
	TryExtractAll:   "Fai clic con il tasto destro sul file > Estrai tutto… per usare l’estrattore di Windows.",

	Extracting: "Estrazione di «%s»",
	Cancel:     "Annulla",
	Stopping:   "Interruzione…",
	Of:         "%s di %s",
	FewSeconds: "mancano pochi secondi",
	Seconds:    "mancano circa %d secondi",
	Minute:     "manca circa un minuto",
	Minutes:    "mancano circa %d minuti",
	Bytes:      "byte",
	Units:      [4]string{"KB", "MB", "GB", "TB"},
	Decimal:    ",",

	VerbExtract:     "Estrai",
	VerbExtractHere: "Estrai qui (ZipClick)",
	TypeName:        "Archivio ZIP",
	AppDescription:  "Estrai i file .zip con un doppio clic, come su un Mac.",
}
