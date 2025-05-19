package gofrr

// StatusCode represents the status code of a command execution.
type StatusCode byte

// status codes are taken from frr lib/command.h
const (
	Success StatusCode = iota
	Warning
	ErrNoMatch
	ErrAmbiguous
	ErrIncomplete
	ErrExeedArgcMax
	ErrNothingTodo
	CompleteFullMatch
	CompleteMatch
	CompleteListMatch
	SuccessDaemon
	ErrNoFile
	Suspend
	WarningConfigFailed
	NotMyInstance
	NoLevelUp
	ErrNoDaemon
)

//nolint:cyclop // ignore too many lines
func (sc StatusCode) String() string {
	switch sc {
	case Success:
		return "CMD_SUCCESS"
	case Warning:
		return "CMD_WARNING"
	case ErrNoMatch:
		return "CMD_ERR_NO_MATCH"
	case ErrAmbiguous:
		return "CMD_ERR_AMBIGUOUS"
	case ErrIncomplete:
		return "CMD_ERR_INCOMPLETE"
	case ErrExeedArgcMax:
		return "CMD_ERR_EXEED_ARGC_MAX"
	case ErrNothingTodo:
		return "CMD_ERR_NOTHING_TODO"
	case CompleteFullMatch:
		return "CMD_COMPLETE_FULL_MATCH"
	case CompleteMatch:
		return "CMD_COMPLETE_MATCH"
	case CompleteListMatch:
		return "CMD_COMPLETE_LIST_MATCH"
	case SuccessDaemon:
		return "CMD_SUCCESS_DAEMON"
	case ErrNoFile:
		return "CMD_ERR_NO_FILE"
	case Suspend:
		return "CMD_SUSPEND"
	case WarningConfigFailed:
		return "CMD_WARNING_CONFIG_FAILED"
	case NotMyInstance:
		return "CMD_NOT_MY_INSTANCE"
	case NoLevelUp:
		return "CMD_NO_LEVEL_UP"
	case ErrNoDaemon:
		return "CMD_ERR_NO_DAEMON"
	default:
		return "unknown status code"
	}
}
