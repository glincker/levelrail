package dbviewer

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// Output protocol: one JSON document per line; mongoMoreMark means more rows
// existed than the limit, mongoErrPrefix carries the server's error text.
const (
	mongoMoreMark   = "LRMORE"
	mongoDoneMark   = "LRDONE"
	mongoErrPrefix  = "LRERR:"
	mongoLastPrefix = "LRLAST:"
)

// MongoResult holds the lines one Mongo operation produced.
type MongoResult struct {
	Documents []json.RawMessage `json:"documents"`
	Truncated bool              `json:"truncated"`
	// NextAfter is the canonical extended JSON _id of the last document when more exist.
	NextAfter  json.RawMessage `json:"next_after,omitempty"`
	DurationMs int64           `json:"duration_ms"`
}

// mongoCmd runs mongosh in the container. Credentials come from the
// container's own environment inside the script, never from an argument.
func mongoCmd(timeout time.Duration) []string {
	// timeout(1) makes a stuck exec end inside the container, not only on our side.
	secs := max(1, int((timeout+time.Second-1)/time.Second))
	return []string{"sh", "-c", fmt.Sprintf("exec timeout -k 2 %d mongosh --quiet --norc --file /dev/stdin", secs)}
}

func jsLit(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func jsParse(canonical string) string {
	return "EJSON.parse(" + jsLit(canonical) + ")"
}

// mongoScript assembles the script from validated parts and literals only.
func mongoScript(req MongoRequest, p mongoParts) string {
	var b strings.Builder
	b.WriteString(`(function(){try{`)
	b.WriteString(`if(process.env.MONGO_INITDB_ROOT_USERNAME){db.getSiblingDB("admin").auth(process.env.MONGO_INITDB_ROOT_USERNAME,process.env.MONGO_INITDB_ROOT_PASSWORD);}`)
	fmt.Fprintf(&b, `var MS=%d,CELL=%d,L=%d,SR=%d;`, p.lim.Timeout.Milliseconds(), p.lim.MaxCellBytes, p.limit, p.lim.SchemaRows)
	b.WriteString(`var E=function(o){return EJSON.stringify(o,null,0,{relaxed:true});};`)
	b.WriteString(`var P=function(o){var s=E(o);if(s.length>CELL){var i=E(o._id);print(i!==undefined&&i.length*4<CELL?E({_id:o._id,"__truncated":true,bytes:s.length}):E({"__truncated":true,bytes:s.length}));}else{print(s);}};`)
	b.WriteString(`var EC=function(o){return EJSON.stringify(o,null,0,{relaxed:false});};`)
	b.WriteString(`var DRAIN=function(cur){var n=0,last;while(cur.hasNext()){var d=cur.next();n++;if(n>L){print("` + mongoMoreMark + `");var c=last===undefined?undefined:EC(last);if(c!==undefined&&c.length*4<CELL){print("` + mongoLastPrefix + `"+c);}break;}last=d._id;P(d);}};`)
	if req.Database != "" {
		fmt.Fprintf(&b, `var D=db.getSiblingDB(%s);`, jsLit(req.Database))
	}
	if req.Collection != "" {
		fmt.Fprintf(&b, `var C=D.getCollection(%s);`, jsLit(req.Collection))
	}
	switch req.Op {
	case MongoListDatabases:
		b.WriteString(`db.getMongo().getDBs().databases.forEach(function(x){print(JSON.stringify({name:x.name,sizeOnDisk:Number(x.sizeOnDisk),empty:!!x.empty}));});`)
	case MongoListCollections:
		b.WriteString(`D.getCollectionInfos({},true).slice(0,SR).forEach(function(x){print(JSON.stringify({name:x.name,type:x.type}));});`)
	case MongoCollections:
		b.WriteString(mongoOverviewScript(req.Database))
	case MongoFind:
		after := ""
		if p.after != "" {
			op := "$gt"
			if p.sort == `{"_id":-1}` {
				op = "$lt"
			}
			after = fmt.Sprintf(`F={$and:[F,{_id:{%s:%s}}]};`, op, jsParse(p.after))
		}
		sortSpec := p.sort
		if sortSpec == "{}" {
			sortSpec = `{"_id":1}`
		}
		fmt.Fprintf(&b, `var F=%s;%sDRAIN(C.find(F,%s).sort(%s).limit(L+1).maxTimeMS(MS));`,
			jsParse(p.filter), after, jsParse(p.proj), jsParse(sortSpec))
	case MongoCountDocuments:
		fmt.Fprintf(&b, `print(JSON.stringify({count:Number(C.countDocuments(%s,{maxTimeMS:MS}))}));`, jsParse(p.filter))
	case MongoEstimatedCount:
		b.WriteString(`print(JSON.stringify({count:Number(C.estimatedDocumentCount({maxTimeMS:MS}))}));`)
	case MongoGetIndexes:
		b.WriteString(`C.getIndexes().forEach(function(x){P(x);});`)
	case MongoStats:
		b.WriteString(`var S=D.runCommand({collStats:` + jsLit(req.Collection) + `});print(JSON.stringify({count:Number(S.count),size:Number(S.size),storageSize:Number(S.storageSize),avgObjSize:Number(S.avgObjSize||0),nindexes:Number(S.nindexes),totalIndexSize:Number(S.totalIndexSize)}));`)
	}
	b.WriteString(`print("` + mongoDoneMark + `");}catch(e){print("` + mongoErrPrefix + `"+String(e.message||e));}})()` + "\n")
	return b.String()
}

// mongoOverviewScript lists databases, collections, counts, sizes and indexes
// with listing, count, stats and index reads only.
func mongoOverviewScript(database string) string {
	dbs := `DBS.map(function(x){return x.name;}).filter(function(n){return n!=="admin"&&n!=="config"&&n!=="local";})`
	if database != "" {
		dbs = `[` + jsLit(database) + `]`
	}
	return `var left=SR,DBS=db.getMongo().getDBs().databases;` + dbs + `.forEach(function(dn){if(left<=0){return;}var d=db.getSiblingDB(dn);` +
		`print(JSON.stringify({kind:"db",name:dn,sizeOnDisk:Number((DBS.filter(function(x){return x.name===dn;})[0]||{}).sizeOnDisk||0)}));` +
		`d.runCommand({listCollections:1,nameOnly:true,maxTimeMS:MS,cursor:{batchSize:1048576}}).cursor.firstBatch.forEach(function(ci){if(left<=0||ci.name.toLowerCase().indexOf("system.")===0){return;}left--;` +
		`var o={kind:"collection",db:dn,name:ci.name,type:ci.type,count:0,size:0,storageSize:0,indexes:[]};` +
		`try{if(ci.type==="collection"){var c=d.getCollection(ci.name);o.count=Number(c.estimatedDocumentCount({maxTimeMS:MS}));` +
		`var s=d.runCommand({collStats:ci.name,maxTimeMS:MS});o.size=Number(s.size||0);o.storageSize=Number(s.storageSize||0);` +
		`o.indexes=c.getIndexes().map(function(x){return {name:x.name,key:x.key,unique:!!x.unique};});}}catch(e){o.error=String(e.message||e);}` +
		`print(JSON.stringify(o));});});if(left<=0){print("` + mongoMoreMark + `");}`
}

// RunMongo executes a guard-approved query and collects its output lines.
func (t Target) RunMongo(ctx context.Context, q MongoQuery) (MongoResult, error) {
	ctx, cancel := context.WithTimeout(ctx, t.Limits.Timeout+timeoutMargin)
	defer cancel()

	start := time.Now()
	rc, err := t.Exec.ExecWithInput(ctx, t.ContainerID, mongoCmd(t.Limits.Timeout), strings.NewReader(q.script))
	if err != nil {
		return MongoResult{}, wrapExecError(ctx, err)
	}
	defer func() { _ = rc.Close() }()

	maxRows := t.Limits.MaxRows
	if q.Op == MongoCollections || q.Op == MongoListCollections {
		maxRows = t.Limits.SchemaRows + 1
	}
	capped := &capReader{r: rc, left: t.Limits.MaxBytes}
	sc := bufio.NewScanner(capped)
	// A cell is capped in the script at MaxCellBytes characters; allow for multibyte text.
	sc.Buffer(make([]byte, 0, 64<<10), 4*t.Limits.MaxCellBytes+4096)
	res := MongoResult{Documents: []json.RawMessage{}}
	sawDone := false
	for sc.Scan() {
		// Work on the scanner's bytes and copy only the lines kept as documents.
		line := bytes.TrimSpace(sc.Bytes())
		switch {
		case len(line) == 0:
		case string(line) == mongoMoreMark:
			res.Truncated = true
		case string(line) == mongoDoneMark:
			sawDone = true
		case bytes.HasPrefix(line, []byte(mongoLastPrefix)):
			res.NextAfter = json.RawMessage(bytes.Clone(line[len(mongoLastPrefix):]))
		case bytes.HasPrefix(line, []byte(mongoErrPrefix)):
			return MongoResult{}, &QueryError{Message: string(line[len(mongoErrPrefix):])}
		case !json.Valid(line):
			// mongosh banners or warnings, not data.
		case len(res.Documents) >= maxRows:
			res.Truncated = true
		default:
			res.Documents = append(res.Documents, json.RawMessage(bytes.Clone(line)))
		}
		if res.Truncated && len(res.Documents) >= maxRows {
			break
		}
	}
	if capped.tripd {
		res.Truncated = true
	}
	if err := sc.Err(); err != nil {
		if errors.Is(err, bufio.ErrTooLong) {
			res.Truncated = true
		} else if !capped.tripd {
			return MongoResult{}, wrapExecError(ctx, err)
		}
	}
	if !res.Truncated && !sawDone {
		return MongoResult{}, errors.New("dbviewer: mongosh ended before the result was complete")
	}
	if !res.Truncated {
		if _, err := io.Copy(io.Discard, rc); err != nil {
			return MongoResult{}, wrapExecError(ctx, err)
		}
	}
	res.DurationMs = time.Since(start).Milliseconds()
	return res, nil
}
