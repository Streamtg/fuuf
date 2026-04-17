// package commands
// stream.go - Modificado para usar HOST interno y HOST público (Cloudflare)

package commands

import (
	"fmt"
	"strings"

	"EverythingSuckz/fsb/config"
	"EverythingSuckz/fsb/internal/utils"

	"github.com/celestix/gotgproto/dispatcher"
	"github.com/celestix/gotgproto/dispatcher/handlers"
	"github.com/celestix/gotgproto/ext"
	"github.com/celestix/gotgproto/storage"
	"github.com/celestix/gotgproto/types"
	"github.com/gotd/td/telegram/message/styling"
	"github.com/gotd/td/tg"
)

func (m *command) LoadStream(dispatcher dispatcher.Dispatcher) {
	log := m.log.Named("start")
	defer log.Sugar().Info("Loaded")
	dispatcher.AddHandler(
		handlers.NewMessage(nil, sendLink),
	)
}

func supportedMediaFilter(m *types.Message) (bool, error) {
	if not := m.Media == nil; not {
		return false, dispatcher.EndGroups
	}
	switch m.Media.(type) {
	case *tg.MessageMediaDocument:
		return true, nil
	case *tg.MessageMediaPhoto:
		return true, nil
	case tg.MessageMediaClass:
		return false, dispatcher.EndGroups
	default:
		return false, nil
	}
}

func sendLink(ctx *ext.Context, u *ext.Update) error {
	chatId := u.EffectiveChat().GetID()
	peerChatId := ctx.PeerStorage.GetPeerById(chatId)
	if peerChatId.Type != int(storage.TypeUser) {
		return dispatcher.EndGroups
	}
	if len(config.ValueOf.AllowedUsers) != 0 && !utils.Contains(config.ValueOf.AllowedUsers, chatId) {
		ctx.Reply(u, ext.ReplyTextString("You are not allowed to use this bot."), nil)
		return dispatcher.EndGroups
	}
	supported, err := supportedMediaFilter(u.EffectiveMessage)
	if err != nil {
		return err
	}
	if !supported {
		ctx.Reply(u, ext.ReplyTextString("Sorry, this message type is unsupported."), nil)
		return dispatcher.EndGroups
	}
	update, err := utils.ForwardMessages(ctx, chatId, config.ValueOf.LogChannelID, u.EffectiveMessage.ID)
	if err != nil {
		utils.Logger.Sugar().Error(err)
		ctx.Reply(u, ext.ReplyTextString(fmt.Sprintf("Error - %s", err.Error())), nil)
		return dispatcher.EndGroups
	}
	if len(update.Updates) < 2 {
		ctx.Reply(u, ext.ReplyTextString("Error - unexpected update structure from Telegram"), nil)
		return dispatcher.EndGroups
	}
	msgIDUpdate, ok := update.Updates[0].(*tg.UpdateMessageID)
	if !ok {
		ctx.Reply(u, ext.ReplyTextString("Error - unexpected update type"), nil)
		return dispatcher.EndGroups
	}
	messageID := msgIDUpdate.ID
	newMsg, ok := update.Updates[1].(*tg.UpdateNewChannelMessage)
	if !ok {
		ctx.Reply(u, ext.ReplyTextString("Error - unexpected channel message update"), nil)
		return dispatcher.EndGroups
	}
	msg, ok := newMsg.Message.(*tg.Message)
	if !ok {
		ctx.Reply(u, ext.ReplyTextString("Error - unexpected message type"), nil)
		return dispatcher.EndGroups
	}
	doc := msg.Media
	file, err := utils.FileFromMedia(doc)
	if err != nil {
		ctx.Reply(u, ext.ReplyTextString(fmt.Sprintf("Error - %s", err.Error())), nil)
		return dispatcher.EndGroups
	}

	fullHash := utils.PackFile(
		file.FileName,
		file.FileSize,
		file.MimeType,
		file.ID,
	)
	hash := utils.GetShortHash(fullHash)

	// ── HOST interno: lo usa el bot para construir el path
	//    Ejemplo: http://127.0.0.1:8080
	internalHost := config.ValueOf.Host

	// ── HOST público: lo ven los usuarios (Cloudflare Worker)
	//    Ejemplo: https://tu-worker.workers.dev
	//    Si no está configurado, usa el interno como fallback
	publicHost := config.ValueOf.PublicHost
	if publicHost == "" {
		publicHost = internalHost
	}

	// Path del stream (igual para ambos)
	streamPath := fmt.Sprintf("/stream/%d?hash=%s", messageID, hash)

	// Link interno: usado por el bot para verificar/procesar
	// (no se muestra al usuario)
	_ = fmt.Sprintf("%s%s", internalHost, streamPath)

	// Link público: lo que ven los usuarios
	publicLink := fmt.Sprintf("%s%s", publicHost, streamPath)

	// Texto que se muestra: solo el link de Cloudflare
	text := styling.Code(publicLink)

	// Botones con links públicos
	row := tg.KeyboardButtonRow{
		Buttons: []tg.KeyboardButtonClass{
			&tg.KeyboardButtonURL{
				Text: "⬇️ Download",
				URL:  publicLink + "&d=true",
			},
		},
	}

	// Botón de stream para video/audio/pdf
	if strings.Contains(file.MimeType, "video") ||
		strings.Contains(file.MimeType, "audio") ||
		strings.Contains(file.MimeType, "pdf") {
		row.Buttons = append(row.Buttons, &tg.KeyboardButtonURL{
			Text: "▶️ Stream",
			URL:  publicLink,
		})
	}

	markup := &tg.ReplyInlineMarkup{
		Rows: []tg.KeyboardButtonRow{row},
	}

	// Si es localhost no mostrar botones (modo desarrollo)
	if strings.Contains(publicLink, "localhost") ||
		strings.Contains(publicLink, "127.0.0.1") {
		_, err = ctx.Reply(u, ext.ReplyTextStyledText(text), &ext.ReplyOpts{
			NoWebpage:        false,
			ReplyToMessageId: u.EffectiveMessage.ID,
		})
	} else {
		_, err = ctx.Reply(u, ext.ReplyTextStyledText(text), &ext.ReplyOpts{
			Markup:           markup,
			NoWebpage:        false,
			ReplyToMessageId: u.EffectiveMessage.ID,
		})
	}

	if err != nil {
		utils.Logger.Sugar().Error(err)
		ctx.Reply(u, ext.ReplyTextString(fmt.Sprintf("Error - %s", err.Error())), nil)
	}

	return dispatcher.EndGroups
}
