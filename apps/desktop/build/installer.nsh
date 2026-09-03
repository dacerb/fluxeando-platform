; La desinstalación conserva los datos por defecto y pide confirmación explícita
; antes de borrar tanto el perfil local como la base SQLite elegida por la persona.
!macro customUnInstall
  MessageBox MB_YESNO|MB_ICONQUESTION|MB_DEFBUTTON2 "¿También querés eliminar todos los datos locales de CashFlow? Esto borra la sesión, configuración, cachés y la base SQLite seleccionada. Esta acción no se puede deshacer." IDNO keepCashFlowData

  ReadRegStr $0 HKCU "Software\CashFlow" "LocalDatabasePath"
  StrCmp $0 "" skipExternalDatabase
  Delete "$0"
  Delete "$0-wal"
  Delete "$0-shm"

skipExternalDatabase:
  RMDir /r "$APPDATA\@cashflow\desktop"
  RMDir /r "$LOCALAPPDATA\@cashflow\desktop"
  DeleteRegKey HKCU "Software\CashFlow"

keepCashFlowData:
!macroend
