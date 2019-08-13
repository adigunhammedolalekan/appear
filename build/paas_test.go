package build

import (
	"fmt"
	"regexp"
	"testing"
)

func TestDockerService_PushImage(t *testing.T) {
	s := `Dear HAMMED OLALEKAN ADIGUN,
	NGN 2,050.00
	
	Credit alert on XXXXXX0691
	
	Description: REV MB TRF FBN . NIBSS/0019430691
	
	This transaction took place on 7/29/2019 7:02:00 PM.
	
	   How do you feel about this service?
	dfvf     ascacdac
	For any queries please call
	0700 CALL STANBIC (0700 2255 7826242) or +234 1 4222222
	
	EMAIL:
	customercarenigeria@stanbicibtc.com
	ACCOUNT DETAILS
	Transaction Reference:
	S448548/ 771
	
	Current Balance:
	NGN 135,453.98
	
	Your Branch:
	IWO TOWN
	Please note that this transaction may not reflect in your balance until it is cleared.`

	// regular expression pattern
	regE := regexp.MustCompile("NGN [A-Za-z0-9,.]*")
	fmt.Println(regE.FindAllString(s, -1))
}
