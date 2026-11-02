package integration

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/xo/oracle"
	"github.com/xo/oracle/aq"
)

// createQueue creates the queue name of the payload type payload, and
// returns a func that drops it. A queue of JSON goes in the tablespace USERS
// when the user has SYSTEM, which holds no JSON.
func createQueue(t *testing.T, db *sql.DB, name, payload string) func() {
	t.Helper()
	drop := func() {
		db.Exec(fmt.Sprintf(`BEGIN
	DBMS_AQADM.STOP_QUEUE('%[1]s');
	DBMS_AQADM.DROP_QUEUE('%[1]s');
	DBMS_AQADM.DROP_QUEUE_TABLE('%[1]s_T', TRUE);
END;`, name))
		db.Exec(fmt.Sprintf("BEGIN DBMS_AQADM.DROP_QUEUE_TABLE('%s_T', TRUE); END;", name))
	}
	drop()
	storage := ""
	var space string
	if err := db.QueryRowContext(t.Context(), "SELECT default_tablespace FROM user_users").Scan(&space); err == nil && payload == "JSON" && space == "SYSTEM" {
		storage = "TABLESPACE USERS"
	}
	if _, err := db.ExecContext(t.Context(), fmt.Sprintf(`BEGIN
	DBMS_AQADM.CREATE_QUEUE_TABLE('%[1]s_T', '%[2]s', storage_clause => '%[3]s');
	DBMS_AQADM.CREATE_QUEUE('%[1]s', '%[1]s_T');
	DBMS_AQADM.START_QUEUE('%[1]s');
END;`, name, payload, storage)); err != nil {
		t.Fatalf("creating the queue %s: %v", name, err)
	}
	return drop
}

// queueConn returns a connection and the queue name on it.
func queueConn(t *testing.T, db *sql.DB, name string, kind aq.MessageType, udt string) (*sql.Conn, aq.Queue) {
	t.Helper()
	conn, err := db.Conn(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	q, err := aq.CreateQueue(conn, name, kind, udt)
	if err != nil {
		conn.Close()
		t.Fatal(err)
	}
	return conn, q
}

// noWait is the options of a dequeue that waits for no message.
func noWait() *aq.DequeueOptions {
	opts := aq.DefaultDequeueOptions()
	opts.Wait = 0
	return opts
}

// TestAQRaw enqueues and dequeues RAW messages. It holds cases of
// tests/aq/test_1400_raw.py of python-oracledb, written again here: an empty
// queue (1400), messages in order (1401), the visibility of an enqueue
// (1407, 1408), the delivery modes (1409 to 1411), and the ID, the time and
// the priority of a message (1414, 1415).
func TestAQRaw(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	defer createQueue(t, db, "GO_RAW_Q", "RAW")()
	conn, q := queueConn(t, db, "GO_RAW_Q", aq.RAW, "")
	defer conn.Close()
	q.SetAutoCommit(true)

	t.Run("1400 an empty queue", func(t *testing.T) {
		// go-ora returned ORA-25228 and an empty message
		msg, err := q.Dequeue(noWait())
		if err != nil || msg != nil {
			t.Errorf("got %+v, %v", msg, err)
		}
	})

	t.Run("1401 messages in order", func(t *testing.T) {
		payloads := []string{"The first message", "The second message", "The third message"}
		for i, p := range payloads {
			msg, err := q.NewMessage([]byte(p))
			if err != nil {
				t.Fatal(err)
			}
			msg.Priority = i
			if err := q.Enqueue(msg); err != nil {
				t.Fatalf("enqueue %d: %v", i, err)
			}
		}
		for i, p := range payloads {
			msg, err := q.Dequeue(noWait())
			if err != nil {
				t.Fatalf("dequeue %d: %v", i, err)
			}
			if b, _ := msg.Payload.([]byte); !bytes.Equal(b, []byte(p)) {
				t.Errorf("dequeue %d: got %q, want %q", i, msg.Payload, p)
			}
			if len(msg.ServerMessageID()) != 16 || msg.EnqTime.IsZero() || msg.Priority != i {
				t.Errorf("dequeue %d: the ID %x, the time %v and the priority %d", i, msg.ServerMessageID(), msg.EnqTime, msg.Priority)
			}
		}
		if msg, err := q.Dequeue(noWait()); err != nil || msg != nil {
			t.Errorf("after: got %+v, %v", msg, err)
		}
	})

	t.Run("1408 an enqueue visible at once", func(t *testing.T) {
		q.SetAutoCommit(false)
		defer q.SetAutoCommit(true)
		msg, err := q.NewMessage([]byte("visible"))
		if err != nil {
			t.Fatal(err)
		}
		msg.VisibilityMode = aq.VisibilityImmediate
		if err := q.Enqueue(msg); err != nil {
			t.Fatal(err)
		}
		other, oq := queueConn(t, db, "GO_RAW_Q", aq.RAW, "")
		defer other.Close()
		oq.SetAutoCommit(true)
		got, err := oq.Dequeue(noWait())
		if b, _ := got.Payload.([]byte); err != nil || string(b) != "visible" {
			t.Errorf("got %+v, %v", got, err)
		}
	})

	t.Run("1407 an enqueue visible at commit", func(t *testing.T) {
		q.SetAutoCommit(false)
		defer q.SetAutoCommit(true)
		msg, err := q.NewMessage([]byte("on commit"))
		if err != nil {
			t.Fatal(err)
		}
		if err := q.Enqueue(msg); err != nil {
			t.Fatal(err)
		}
		other, oq := queueConn(t, db, "GO_RAW_Q", aq.RAW, "")
		defer other.Close()
		oq.SetAutoCommit(true)
		if got, err := oq.Dequeue(noWait()); err != nil || got != nil {
			t.Errorf("before the commit: got %+v, %v", got, err)
		}
		if _, err := conn.ExecContext(t.Context(), "COMMIT"); err != nil {
			t.Fatal(err)
		}
		got, err := oq.Dequeue(noWait())
		if b, _ := got.Payload.([]byte); err != nil || string(b) != "on commit" {
			t.Errorf("after the commit: got %+v, %v", got, err)
		}
	})

	t.Run("1409 a buffered message", func(t *testing.T) {
		msg, err := q.NewMessage([]byte("buffered"))
		if err != nil {
			t.Fatal(err)
		}
		msg.DeliveryMode = aq.DeliveryModeBuffered
		msg.VisibilityMode = aq.VisibilityImmediate
		if err := q.Enqueue(msg); err != nil {
			t.Fatal(err)
		}
		opts := noWait()
		opts.Delivery = aq.DeliveryModeBuffered
		opts.Visibility = aq.VisibilityImmediate
		got, err := q.Dequeue(opts)
		if b, _ := got.Payload.([]byte); err != nil || string(b) != "buffered" {
			t.Errorf("got %+v, %v", got, err)
		}
	})
}

// TestAQBulk enqueues and dequeues messages in one call, as 1200, 1201 and
// 1203 of tests/aq/test_1200_bulk.py of python-oracledb do.
func TestAQBulk(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	defer createQueue(t, db, "GO_BULK_Q", "RAW")()
	conn, q := queueConn(t, db, "GO_BULK_Q", aq.RAW, "")
	defer conn.Close()
	q.SetAutoCommit(true)
	// go-ora returned 5 empty messages and a reset, which it called a
	// timeout
	if msgs, err := q.DequeueMessages(noWait(), 5); err != nil || len(msgs) != 0 {
		t.Errorf("an empty queue: got %d messages, %v", len(msgs), err)
	}
	var want []string
	var msgs []*aq.Message
	for i := range 12 {
		p := fmt.Sprintf("The bulk message %d", i)
		want = append(want, p)
		msg, err := q.NewMessage([]byte(p))
		if err != nil {
			t.Fatal(err)
		}
		msgs = append(msgs, msg)
	}
	if err := q.EnqueueMessages(msgs); err != nil {
		t.Fatal(err)
	}
	var got []string
	for range 3 {
		out, err := q.DequeueMessages(noWait(), 5)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range out {
			b, _ := m.Payload.([]byte)
			got = append(got, string(b))
		}
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

// aqObject is the payload of the queue GO_UDT_Q.
type aqObject struct {
	Name   string  `udt:"NAME"`
	Amount float64 `udt:"AMOUNT"`
}

// TestAQObject enqueues and dequeues objects, as 1000 and 1001 of
// tests/aq/test_1000_dbobject.py of python-oracledb do.
func TestAQObject(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.Exec("DROP TYPE go_aq_obj FORCE")
	if err := execCmd(db, "CREATE TYPE go_aq_obj AS OBJECT (name VARCHAR2(30), amount NUMBER)"); err != nil {
		t.Fatal(err)
	}
	defer db.Exec("DROP TYPE go_aq_obj FORCE")
	defer createQueue(t, db, "GO_UDT_Q", "GO_AQ_OBJ")()
	if err := oracle.RegisterType(db, "GO_AQ_OBJ", "", aqObject{}); err != nil {
		t.Fatal(err)
	}
	conn, q := queueConn(t, db, "GO_UDT_Q", aq.UDT, "GO_AQ_OBJ")
	defer conn.Close()
	q.SetAutoCommit(true)
	in := []aqObject{{"The first", 1.5}, {"The second", 2.25}}
	for _, v := range in {
		msg, err := q.NewMessage(v)
		if err != nil {
			t.Fatal(err)
		}
		if err := q.Enqueue(msg); err != nil {
			t.Fatal(err)
		}
	}
	for i, want := range in {
		msg, err := q.Dequeue(noWait())
		if err != nil {
			t.Fatal(err)
		}
		if got, ok := msg.Payload.(aqObject); !ok || got != want {
			t.Errorf("dequeue %d: got %#v, want %#v", i, msg.Payload, want)
		}
	}
}

// TestAQJSON enqueues and dequeues JSON, as 1600 and 1601 of
// tests/aq/test_1600_json.py of python-oracledb do. A JSON queue needs 21c.
func TestAQJSON(t *testing.T) {
	db, err := getDB(t)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	requireRelease(t, db, 21)
	defer createQueue(t, db, "GO_JSON_Q", "JSON")()
	conn, q := queueConn(t, db, "GO_JSON_Q", aq.JSON, "")
	defer conn.Close()
	q.SetAutoCommit(true)
	in := map[string]any{"name": "A JSON message", "count": 3.0, "tags": []any{"a", "b"}}
	msg, err := q.NewMessage(in)
	if err != nil {
		t.Fatal(err)
	}
	if err := q.Enqueue(msg); err != nil {
		t.Fatal(err)
	}
	out, err := q.Dequeue(noWait())
	if err != nil {
		t.Fatal(err)
	}
	want, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(jsonValue(t, out.Payload)); !sameJSON(got, string(want)) {
		t.Errorf("got %s, want %s", got, want)
	}
}

// jsonValue returns the value of a JSON payload.
func jsonValue(t *testing.T, v any) any {
	t.Helper()
	switch j := v.(type) {
	case oracle.JSON:
		val, err := j.Value()
		if err != nil {
			t.Fatal(err)
		}
		return val
	case *oracle.JSON:
		val, err := j.Value()
		if err != nil {
			t.Fatal(err)
		}
		return val
	}
	return v
}
