package main

import (
	"bufio"
	"strings"
	"time"

	"flag"
	"os"

	"github.com/PagerDuty/go-pagerduty"
	"github.com/adlio/trello"
	log "github.com/sirupsen/logrus"
)

var (
	configuration = map[string]string{
		"trelloAppKey":   "overwrite",
		"trelloToken":    "overwrite",
		"trelloUserName": "overwrite",
		"trelloCardID":   "overwrite",
		"pgtoken":        "overwrite",
		"pdScheduleName": "overwrite",
	}
)

// PDOnCallUser collects all information needed to set the PD Oncall User for the next 24h
type PDOnCallUser struct {
	TrelloName       string
	PDUserID         string
	PDSchudule       string
	PDTrelloUserName string
	trelloClient     *trello.Client
	pdclient         *pagerduty.Client
}

func getCard(client *trello.Client) *trello.Card {
	card, err := client.GetCard(configuration["cardID"], trello.Defaults())
	if err != nil {
		log.Fatalf("cannot get card: %v", err)

	}
	return card
}

func (m *PDOnCallUser) getUsersFromCard() {
	card := getCard(m.trelloClient)
	members, err := card.GetMembers(trello.Defaults())

	if err != nil {
		log.Fatalf("can not get members from card: %v", err)

	}
	if len(members) < 1 {
		log.Fatalf("No SRE Users on Trello Board, bailing out...")
	}
	m.TrelloName = members[0].Username

}

func init() {

	//networked := flag.Bool("networked", false, "get remote config")
	//netname = flag.String("netname", "chars", "Metric {chars|words|lines};.")
	debugset := flag.Bool("debug", false, "turn the noise on")
	config := flag.String("configfile", "pagerduty.cfg", "Path to configuration file")
	token := flag.String("tokenfile", ".token", "Path to API token and key file")
	flag.Parse()
	if *debugset {
		log.SetLevel(log.DebugLevel)
	}
	if *config != "" {
		readConfigFromFile(*config, configuration)

	}
	if *token != "" {
		readConfigFromFile(*token, configuration)
	}

	return

}

/*
  Replace with appropriate Library
*/
func readConfigFromFile(filename string, conf map[string]string) {
	if filename == "" {
		return
	}

	file, err := os.Open(filename)
	if err != nil {
		log.Fatal(err)
	}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		a := strings.Split(string(scanner.Text()), "=")
		_, ok := conf[a[0]]
		if a[0] != "" && ok {
			conf[strings.Trim(a[0], " ")] = strings.Trim(a[1], " ")
			log.Debugf("Replacing configuration with: %v", scanner.Text())
		}

	}

}

/*
  EOF Replace with appropriate Library
*/
func getNow() string {

	layout := "2006-01-02T15:04:05"
	x := time.Now()
	return x.Format(layout)
}

func getTomorrow() string {
	layout := "2006-01-02T15:04:05"
	x := time.Now()
	x = x.Add(time.Hour * 24)
	return x.Format(layout)
}

func (m *PDOnCallUser) getPdScheduleUsers() {
	schedules, err := m.pdclient.ListSchedules(pagerduty.ListSchedulesOptions{})
	if err != nil {
		log.Fatalf("cannot get schedule %v", err)
	}
	for sch := range schedules.Schedules {
		if schedules.Schedules[sch].Name == configuration["pdScheduleName"] {
			listOnCallOpts := pagerduty.ListOnCallOptions{}
			listOnCallOpts.Since = getNow()
			listOnCallOpts.Until = getTomorrow()
			listOnCallOpts.TimeZone = "Europe/Berlin"
			listOnCallOpts.ScheduleIDs = []string{schedules.Schedules[sch].ID}
			oc, err := m.pdclient.ListOnCalls(listOnCallOpts)
			if err != nil {
				log.Fatalf("error %v", err)
			}
			m.PDSchudule = schedules.Schedules[sch].ID
			m.PDUserID = oc.OnCalls[0].User.ID
		}
		if m.PDSchudule == "" {
			log.Fatalf("No schedule found")
		}
		if m.PDUserID == "" {
			log.Fatalf("No on oncall User in schedule")
		}
	}
}

func (m *PDOnCallUser) overWriteOnCallPD() {
	or := pagerduty.Override{}
	or.Start = getNow()
	or.End = getTomorrow()

	or.User = pagerduty.APIObject{ID: m.PDTrelloUserName, Type: "user_reference"}
	newor, err := m.pdclient.CreateOverride(m.PDSchudule, or)
	if err != nil {
		log.Fatalf("could not set overwrite %v", err)
	}
	log.Infof("override: %v", newor)

}
func (m *PDOnCallUser) init() {
	m.trelloClient = trello.NewClient(configuration["trelloAppKey"], configuration["trelloToken"])
	m.pdclient = pagerduty.NewClient(configuration["pgtoken"])
	log.Infof("struct init")
}

func (m *PDOnCallUser) setPDUserIDforTrelloUser() {
	users, err := m.pdclient.ListUsers(pagerduty.ListUsersOptions{})

	if err != nil {
		log.Fatalf("cannot get pg users %v", err)
	}
	for u := range users.Users {
		if users.Users[u].Description == m.TrelloName {
			m.PDTrelloUserName = users.Users[u].ID
			break
		}
	}
	if m.PDTrelloUserName == "" {
		log.Fatalf("Cannot get PD ID for Trello-User: %v", m.TrelloName)
	}

}
func main() {
	pdoncall := PDOnCallUser{}
	(&pdoncall).init()
	(&pdoncall).getUsersFromCard()
	(&pdoncall).getPdScheduleUsers()
	(&pdoncall).setPDUserIDforTrelloUser()

	if pdoncall.PDUserID != pdoncall.PDTrelloUserName {
		(&pdoncall).overWriteOnCallPD()
	}

	log.Infof("last stop for debugging...")

}
