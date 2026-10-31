package ttc

import (
	"errors"
	"fmt"
	"time"

	"github.com/xo/oracle/internal/types"
)

type DataTypeNego struct {
	conn                   *Connection
	MessageCode            uint8
	Server                 *TCPNego
	TypeAndRep             []uint16
	RuntimeTypeAndRep      []uint16
	DataTypeRepFor1100     uint16
	DataTypeRepFor1200     uint16
	CompileTimeCaps        []byte
	RuntimeCap             []byte
	b32kTypeSupported      bool
	supportSessionStateOps bool
	serverTZVersion        int
	clientTZVersion        int
}

const (
	bufferGrow          int    = 2369
	TNSTypeRepNative    uint16 = 0
	TNSTypeRepUniversal uint16 = 1
	TNSTypeRepOracle    uint16 = 10
	TNSDataTypeVBI      uint16 = 15
	TNSDataTypeUB2      uint16 = 25
	TNSDataTypeUB4      uint16 = 26
	TNSDataTypeSB1      uint16 = 27
	TNSDataTypeSB2      uint16 = 28
	TNSDataTypeSB4      uint16 = 29
	TNSDataTypeSword    uint16 = 30
	TNSDataTypeUword    uint16 = 31
	TNSDataTypePtrb     uint16 = 32
	TNSDataTypePtrw     uint16 = 33
	TNSDataTypeTiddef   uint16 = 10
	TNSDataTypeCursor   uint16 = 102
	TNSDataTypeRDD      uint16 = 104
	TNSDataTypeCLOB     uint16 = 112
	TNSDataTypeBLOB     uint16 = 113
	TNSDataTypeBFILE    uint16 = 114
	TNSDataTypeCFILE    uint16 = 115
	TNSDataTypeRSET     uint16 = 116
	TNSDataTypeDCLOB    uint16 = 195
	TNSDataTypeDBLOB    uint16 = 196
	TNSDataTypeDBFILE   uint16 = 197
	TNSDataTypeUROWID   uint16 = 208
	TNSDataTypeExtNamed uint16 = 108
	TNSDataTypeIntNamed uint16 = 109
	TNSDataTypeExtRef   uint16 = 110
	TNSDataTypeIntRef   uint16 = 111
)

func (nego *DataTypeNego) addTypeRep(dty uint16, ndty uint16, rep uint16) {
	nego.TypeAndRep = append(nego.TypeAndRep, dty)
	nego.TypeAndRep = append(nego.TypeAndRep, ndty)
	if ndty != 0 {
		nego.TypeAndRep = append(nego.TypeAndRep, rep)
		nego.TypeAndRep = append(nego.TypeAndRep, 0)
	}
}

func buildTypeNego(nego *TCPNego, conn *Connection) *DataTypeNego {
	result := DataTypeNego{
		conn:        conn,
		MessageCode: 2,
		Server:      nego,
		TypeAndRep:  make([]uint16, 0, bufferGrow),
		CompileTimeCaps: []byte{
			6, 1, 0, 0, 234, 28, 1, 24, 1, 1,
			1, 1, 1, 1, 0, 41, 144, 3, 7, 3,
			0, 1, 0, 235, 1, 63, 5, 1, 0, 0,
			0, 24, 0, 0, 10, 160, 2, 58, 0, 15,
			71, 0, 1, 0, 40, 0, 0, 0, 0, 0,
			0, 0, 2, 53,
		},
		//CompileTimeCaps: []byte{
		//	6, 1, 0, 0, 106, 1, 1, 11,
		//	1, 1, 1, 1, 1, 1, 0, 41,
		//	144, 3, 7, 3, 0, 1, 0, 235,
		//	1, 0, 5, 1, 0, 0, 0, 24,
		//	0, 0, 7, 32, 2, 58, 0, 0,
		//	5, 0, 0, 0, 8,
		//},
		// CompileTimeCaps: []byte{0x6, 0x1, 0x1, 0x1, 0x6f, 0x1, 0x1, 0x10,
		//	0x1, 0x1, 0x1, 0x1, 0x1, 0x1, 0x1, 0x7f,
		//	0xff, 0x3, 0x10, 0x3, 0x3, 0x1, 0x1, 0xff,
		//	0x1, 0xff, 0xff, 0x1, 0xb, 0x1, 0x1, 0xff,
		//	0x1, 0x6, 0xc, 0xe6, 0x1, 0x7f, 0x5, 0xf,
		//	0x7f, 0xd, 0x3, 0, 0x1},

		RuntimeCap: []byte{2, 1, 0, 0, 0, 0, 4 | 2},
		// RuntimeCap:             []byte{2, 1, 0, 0, 18, 0, 87},
		b32kTypeSupported:      false,
		supportSessionStateOps: false,
		clientTZVersion:        0x2B,
	}
	if result.Server != nil && len(result.Server.ServerCompileTimeCaps) > 0 {
		if len(result.Server.ServerCompileTimeCaps) <= 27 || result.Server.ServerCompileTimeCaps[27] == 0 {
			result.CompileTimeCaps[27] = 0
		}
		xmlTypeClientSideDecoding := false
		if len(result.Server.ServerCompileTimeCaps) > 7 {
			if result.Server.ServerCompileTimeCaps[7] >= 8 && xmlTypeClientSideDecoding {
				result.CompileTimeCaps[36] = 4
			} else if result.Server.ServerCompileTimeCaps[7] < 7 {
				result.CompileTimeCaps[36] = 0
			}
		}
		if len(result.Server.ServerRuntimeCaps) < 1 || result.Server.ServerRuntimeCaps[1]&1 != 1 {
			result.RuntimeCap[1] &= 0
		}
		if len(result.Server.ServerRuntimeCaps) > 6 {
			if result.Server.ServerRuntimeCaps[6]&4 == 4 {
				result.RuntimeCap[6] |= 4
				result.b32kTypeSupported = true
			}
			if result.Server.ServerRuntimeCaps[6]&16 == 16 {
				result.supportSessionStateOps = true
			}
			if result.Server.ServerRuntimeCaps[6]&2 == 2 {
				result.RuntimeCap[6] |= 2
			}
		}
		if len(result.Server.ServerCompileTimeCaps) <= 37 || result.Server.ServerCompileTimeCaps[37]&2 != 2 {
			result.CompileTimeCaps[37] = 0
			result.CompileTimeCaps[1] = 0
		}

	}
	result.addTypeRep(types.NCHAR, types.NCHAR, TNSTypeRepUniversal)
	result.addTypeRep(types.NUMBER, types.NUMBER, TNSTypeRepOracle)
	result.addTypeRep(types.LONG, types.LONG, TNSTypeRepUniversal)
	result.addTypeRep(types.DATE, types.DATE, TNSTypeRepOracle)
	result.addTypeRep(types.RAW, types.RAW, TNSTypeRepUniversal)
	result.addTypeRep(types.LongRaw, types.LongRaw, TNSTypeRepUniversal)
	result.addTypeRep(TNSDataTypeUB2, TNSDataTypeUB2, TNSTypeRepUniversal)
	result.addTypeRep(TNSDataTypeUB4, TNSDataTypeUB4, TNSTypeRepUniversal)
	result.addTypeRep(TNSDataTypeSB1, TNSDataTypeSB1, TNSTypeRepUniversal)
	result.addTypeRep(TNSDataTypeSB2, TNSDataTypeSB2, TNSTypeRepUniversal)
	result.addTypeRep(TNSDataTypeSB4, TNSDataTypeSB4, TNSTypeRepUniversal)
	result.addTypeRep(TNSDataTypeSword, TNSDataTypeSword, TNSTypeRepUniversal)
	result.addTypeRep(TNSDataTypeUword, TNSDataTypeUword, TNSTypeRepUniversal)
	result.addTypeRep(TNSDataTypePtrb, TNSDataTypePtrb, TNSTypeRepUniversal)
	result.addTypeRep(TNSDataTypePtrw, TNSDataTypePtrw, TNSTypeRepUniversal)
	result.addTypeRep(TNSDataTypeTiddef, TNSDataTypeTiddef, TNSTypeRepUniversal)
	result.addTypeRep(types.ROWID, types.ROWID, TNSTypeRepUniversal)
	result.addTypeRep(40, 40, TNSTypeRepUniversal)
	result.addTypeRep(41, 41, TNSTypeRepUniversal)
	result.addTypeRep(117, 117, TNSTypeRepUniversal)
	result.addTypeRep(120, 120, TNSTypeRepUniversal)
	result.addTypeRep(290, 290, TNSTypeRepUniversal)
	result.addTypeRep(291, 291, TNSTypeRepUniversal)
	result.addTypeRep(292, 292, TNSTypeRepUniversal)
	result.addTypeRep(293, 293, TNSTypeRepUniversal)
	result.addTypeRep(294, 294, TNSTypeRepUniversal)
	result.addTypeRep(298, 298, TNSTypeRepUniversal)
	result.addTypeRep(299, 299, TNSTypeRepUniversal)
	result.addTypeRep(300, 300, TNSTypeRepUniversal)
	result.addTypeRep(301, 301, TNSTypeRepUniversal)
	result.addTypeRep(302, 302, TNSTypeRepUniversal)
	result.addTypeRep(303, 303, TNSTypeRepUniversal)
	result.addTypeRep(304, 304, TNSTypeRepUniversal)
	result.addTypeRep(305, 305, TNSTypeRepUniversal)
	result.addTypeRep(306, 306, TNSTypeRepUniversal)
	result.addTypeRep(307, 307, TNSTypeRepUniversal)
	result.addTypeRep(308, 308, TNSTypeRepUniversal)
	result.addTypeRep(309, 309, TNSTypeRepUniversal)
	result.addTypeRep(310, 310, TNSTypeRepUniversal)
	result.addTypeRep(311, 311, TNSTypeRepUniversal)
	result.addTypeRep(312, 312, TNSTypeRepUniversal)
	result.addTypeRep(313, 313, TNSTypeRepUniversal)
	result.addTypeRep(315, 315, TNSTypeRepUniversal)
	result.addTypeRep(316, 316, TNSTypeRepUniversal)
	result.addTypeRep(317, 317, TNSTypeRepUniversal)
	result.addTypeRep(318, 318, TNSTypeRepUniversal)
	result.addTypeRep(319, 319, TNSTypeRepUniversal)
	result.addTypeRep(320, 320, TNSTypeRepUniversal)
	result.addTypeRep(321, 321, TNSTypeRepUniversal)
	result.addTypeRep(322, 322, TNSTypeRepUniversal)
	result.addTypeRep(323, 323, TNSTypeRepUniversal)
	result.addTypeRep(327, 327, TNSTypeRepUniversal)
	result.addTypeRep(328, 328, TNSTypeRepUniversal)
	result.addTypeRep(329, 329, TNSTypeRepUniversal)
	result.addTypeRep(331, 331, TNSTypeRepUniversal)
	result.addTypeRep(333, 333, TNSTypeRepUniversal)
	result.addTypeRep(334, 334, TNSTypeRepUniversal)
	result.addTypeRep(335, 335, TNSTypeRepUniversal)
	result.addTypeRep(336, 336, TNSTypeRepUniversal)
	result.addTypeRep(337, 337, TNSTypeRepUniversal)
	result.addTypeRep(338, 338, TNSTypeRepUniversal)
	result.addTypeRep(339, 339, TNSTypeRepUniversal)
	result.addTypeRep(340, 340, TNSTypeRepUniversal)
	result.addTypeRep(341, 341, TNSTypeRepUniversal)
	result.addTypeRep(342, 342, TNSTypeRepUniversal)
	result.addTypeRep(343, 343, TNSTypeRepUniversal)
	result.addTypeRep(344, 344, TNSTypeRepUniversal)
	result.addTypeRep(345, 345, TNSTypeRepUniversal)
	result.addTypeRep(346, 346, TNSTypeRepUniversal)
	result.addTypeRep(348, 348, TNSTypeRepUniversal)
	result.addTypeRep(349, 349, TNSTypeRepUniversal)
	result.addTypeRep(354, 354, TNSTypeRepUniversal)
	result.addTypeRep(355, 355, TNSTypeRepUniversal)
	result.addTypeRep(359, 359, TNSTypeRepUniversal)
	result.addTypeRep(363, 363, TNSTypeRepUniversal)
	result.addTypeRep(380, 380, TNSTypeRepUniversal)
	result.addTypeRep(381, 381, TNSTypeRepUniversal)
	result.addTypeRep(382, 382, TNSTypeRepUniversal)
	result.addTypeRep(383, 383, TNSTypeRepUniversal)
	result.addTypeRep(384, 384, TNSTypeRepUniversal)
	result.addTypeRep(385, 385, TNSTypeRepUniversal)
	result.addTypeRep(386, 386, TNSTypeRepUniversal)
	result.addTypeRep(387, 387, TNSTypeRepUniversal)
	result.addTypeRep(388, 388, TNSTypeRepUniversal)
	result.addTypeRep(389, 389, TNSTypeRepUniversal)
	result.addTypeRep(390, 390, TNSTypeRepUniversal)
	result.addTypeRep(391, 391, TNSTypeRepUniversal)
	result.addTypeRep(393, 393, TNSTypeRepUniversal)
	result.addTypeRep(394, 394, TNSTypeRepUniversal)
	result.addTypeRep(395, 395, TNSTypeRepUniversal)
	result.addTypeRep(396, 396, TNSTypeRepUniversal)
	result.addTypeRep(397, 397, TNSTypeRepUniversal)
	result.addTypeRep(398, 398, TNSTypeRepUniversal)
	result.addTypeRep(399, 399, TNSTypeRepUniversal)
	result.addTypeRep(400, 400, TNSTypeRepUniversal)
	result.addTypeRep(401, 401, TNSTypeRepUniversal)
	result.addTypeRep(404, 404, TNSTypeRepUniversal)
	result.addTypeRep(405, 405, TNSTypeRepUniversal)
	result.addTypeRep(406, 406, TNSTypeRepUniversal)
	result.addTypeRep(407, 407, TNSTypeRepUniversal)
	result.addTypeRep(413, 413, TNSTypeRepUniversal)
	result.addTypeRep(414, 414, TNSTypeRepUniversal)
	result.addTypeRep(415, 415, TNSTypeRepUniversal)
	result.addTypeRep(416, 416, TNSTypeRepUniversal)
	result.addTypeRep(417, 417, TNSTypeRepUniversal)
	result.addTypeRep(418, 418, TNSTypeRepUniversal)
	result.addTypeRep(419, 419, TNSTypeRepUniversal)
	result.addTypeRep(420, 420, TNSTypeRepUniversal)
	result.addTypeRep(421, 421, TNSTypeRepUniversal)
	result.addTypeRep(422, 422, TNSTypeRepUniversal)
	result.addTypeRep(423, 423, TNSTypeRepUniversal)
	result.addTypeRep(424, 424, TNSTypeRepUniversal)
	result.addTypeRep(425, 425, TNSTypeRepUniversal)
	result.addTypeRep(426, 426, TNSTypeRepUniversal)
	result.addTypeRep(427, 427, TNSTypeRepUniversal)
	result.addTypeRep(429, 429, TNSTypeRepUniversal)
	result.addTypeRep(430, 430, TNSTypeRepUniversal)
	result.addTypeRep(431, 431, TNSTypeRepUniversal)
	result.addTypeRep(432, 432, TNSTypeRepUniversal)
	result.addTypeRep(433, 433, TNSTypeRepUniversal)
	result.addTypeRep(449, 449, TNSTypeRepUniversal)
	result.addTypeRep(450, 450, TNSTypeRepUniversal)
	result.addTypeRep(454, 454, TNSTypeRepUniversal)
	result.addTypeRep(455, 455, TNSTypeRepUniversal)
	result.addTypeRep(456, 456, TNSTypeRepUniversal)
	result.addTypeRep(457, 457, TNSTypeRepUniversal)
	result.addTypeRep(458, 458, TNSTypeRepUniversal)
	result.addTypeRep(459, 459, TNSTypeRepUniversal)
	result.addTypeRep(460, 460, TNSTypeRepUniversal)
	result.addTypeRep(461, 461, TNSTypeRepUniversal)
	result.addTypeRep(462, 462, TNSTypeRepUniversal)
	result.addTypeRep(463, 463, TNSTypeRepUniversal)
	result.addTypeRep(466, 466, TNSTypeRepUniversal)
	result.addTypeRep(467, 467, TNSTypeRepUniversal)
	result.addTypeRep(468, 468, TNSTypeRepUniversal)
	result.addTypeRep(469, 469, TNSTypeRepUniversal)
	result.addTypeRep(470, 470, TNSTypeRepUniversal)
	result.addTypeRep(471, 471, TNSTypeRepUniversal)
	result.addTypeRep(472, 472, TNSTypeRepUniversal)
	result.addTypeRep(473, 473, TNSTypeRepUniversal)
	result.addTypeRep(474, 474, TNSTypeRepUniversal)
	result.addTypeRep(475, 475, TNSTypeRepUniversal)
	result.addTypeRep(476, 476, TNSTypeRepUniversal)
	result.addTypeRep(477, 477, TNSTypeRepUniversal)
	result.addTypeRep(478, 478, TNSTypeRepUniversal)
	result.addTypeRep(479, 479, TNSTypeRepUniversal)
	result.addTypeRep(480, 480, TNSTypeRepUniversal)
	result.addTypeRep(481, 481, TNSTypeRepUniversal)
	result.addTypeRep(482, 482, TNSTypeRepUniversal)
	result.addTypeRep(483, 483, TNSTypeRepUniversal)
	result.addTypeRep(484, 484, TNSTypeRepUniversal)
	result.addTypeRep(485, 485, TNSTypeRepUniversal)
	result.addTypeRep(486, 486, TNSTypeRepUniversal)
	result.addTypeRep(490, 490, TNSTypeRepUniversal)
	result.addTypeRep(491, 491, TNSTypeRepUniversal)
	result.addTypeRep(492, 492, TNSTypeRepUniversal)
	result.addTypeRep(493, 493, TNSTypeRepUniversal)
	result.addTypeRep(494, 494, TNSTypeRepUniversal)
	result.addTypeRep(495, 495, TNSTypeRepUniversal)
	result.addTypeRep(496, 496, TNSTypeRepUniversal)
	result.addTypeRep(498, 498, TNSTypeRepUniversal)
	result.addTypeRep(499, 499, TNSTypeRepUniversal)
	result.addTypeRep(500, 500, TNSTypeRepUniversal)
	result.addTypeRep(501, 501, TNSTypeRepUniversal)
	result.addTypeRep(502, 502, TNSTypeRepUniversal)
	result.addTypeRep(509, 509, TNSTypeRepUniversal)
	result.addTypeRep(510, 510, TNSTypeRepUniversal)
	result.addTypeRep(513, 513, TNSTypeRepUniversal)
	result.addTypeRep(514, 514, TNSTypeRepUniversal)
	result.addTypeRep(516, 516, TNSTypeRepUniversal)
	result.addTypeRep(517, 517, TNSTypeRepUniversal)
	result.addTypeRep(518, 518, TNSTypeRepUniversal)
	result.addTypeRep(519, 519, TNSTypeRepUniversal)
	result.addTypeRep(520, 520, TNSTypeRepUniversal)
	result.addTypeRep(521, 521, TNSTypeRepUniversal)
	result.addTypeRep(522, 522, TNSTypeRepUniversal)
	result.addTypeRep(523, 523, TNSTypeRepUniversal)
	result.addTypeRep(524, 524, TNSTypeRepUniversal)
	result.addTypeRep(525, 525, TNSTypeRepUniversal)
	result.addTypeRep(526, 526, TNSTypeRepUniversal)
	result.addTypeRep(527, 527, TNSTypeRepUniversal)
	result.addTypeRep(528, 528, TNSTypeRepUniversal)
	result.addTypeRep(529, 529, TNSTypeRepUniversal)
	result.addTypeRep(530, 530, TNSTypeRepUniversal)
	result.addTypeRep(531, 531, TNSTypeRepUniversal)
	result.addTypeRep(532, 532, TNSTypeRepUniversal)
	result.addTypeRep(533, 533, TNSTypeRepUniversal)
	result.addTypeRep(534, 534, TNSTypeRepUniversal)
	result.addTypeRep(535, 535, TNSTypeRepUniversal)
	result.addTypeRep(536, 536, TNSTypeRepUniversal)
	result.addTypeRep(537, 537, TNSTypeRepUniversal)
	result.addTypeRep(538, 538, TNSTypeRepUniversal)
	result.addTypeRep(539, 539, TNSTypeRepUniversal)
	result.addTypeRep(540, 540, TNSTypeRepUniversal)
	result.addTypeRep(541, 541, TNSTypeRepUniversal)
	result.addTypeRep(542, 542, TNSTypeRepUniversal)
	result.addTypeRep(543, 543, TNSTypeRepUniversal)
	result.addTypeRep(560, 560, TNSTypeRepUniversal)
	result.addTypeRep(562, 562, TNSTypeRepUniversal)
	result.addTypeRep(565, 565, TNSTypeRepUniversal)
	result.addTypeRep(572, 572, TNSTypeRepUniversal)
	result.addTypeRep(573, 573, TNSTypeRepUniversal)
	result.addTypeRep(574, 574, TNSTypeRepUniversal)
	result.addTypeRep(575, 575, TNSTypeRepUniversal)
	result.addTypeRep(576, 576, TNSTypeRepUniversal)
	result.addTypeRep(578, 578, TNSTypeRepUniversal)
	result.addTypeRep(563, 563, TNSTypeRepUniversal)
	result.addTypeRep(564, 564, TNSTypeRepUniversal)
	result.addTypeRep(579, 579, TNSTypeRepUniversal)
	result.addTypeRep(580, 580, TNSTypeRepUniversal)
	result.addTypeRep(581, 581, TNSTypeRepUniversal)
	result.addTypeRep(582, 582, TNSTypeRepUniversal)
	result.addTypeRep(583, 583, TNSTypeRepUniversal)
	result.addTypeRep(584, 584, TNSTypeRepUniversal)
	result.addTypeRep(585, 585, TNSTypeRepUniversal)
	result.addTypeRep(types.BInteger, types.NUMBER, TNSTypeRepOracle)
	result.addTypeRep(types.FLOAT, types.NUMBER, TNSTypeRepOracle)
	result.addTypeRep(types.NullStr, types.NCHAR, TNSTypeRepUniversal)
	result.addTypeRep(types.VarNum, types.NUMBER, TNSTypeRepOracle)
	result.addTypeRep(types.PDN, types.NUMBER, TNSTypeRepOracle)
	result.addTypeRep(types.VARCHAR, types.NCHAR, TNSTypeRepUniversal)
	result.addTypeRep(13, 0, TNSTypeRepNative)
	result.addTypeRep(14, 0, TNSTypeRepNative)
	result.addTypeRep(TNSDataTypeVBI, 23, TNSTypeRepUniversal)
	result.addTypeRep(16, 0, TNSTypeRepNative)
	result.addTypeRep(17, 0, TNSTypeRepNative)
	result.addTypeRep(18, 0, TNSTypeRepNative)
	result.addTypeRep(19, 0, TNSTypeRepNative)
	result.addTypeRep(20, 0, TNSTypeRepNative)
	result.addTypeRep(21, 0, TNSTypeRepNative)
	result.addTypeRep(22, 0, TNSTypeRepNative)
	result.addTypeRep(39, 646, TNSTypeRepUniversal)
	result.addTypeRep(58, 0, TNSTypeRepNative)
	result.addTypeRep(68, 2, TNSTypeRepOracle)
	result.addTypeRep(69, 0, TNSTypeRepNative)
	result.addTypeRep(70, 0, TNSTypeRepNative)
	result.addTypeRep(74, 0, TNSTypeRepNative)
	result.addTypeRep(76, 0, TNSTypeRepNative)
	result.addTypeRep(91, 2, TNSTypeRepOracle)
	result.addTypeRep(94, 1, TNSTypeRepUniversal)
	result.addTypeRep(95, 23, TNSTypeRepUniversal)
	result.addTypeRep(96, 96, TNSTypeRepUniversal)
	result.addTypeRep(97, 96, TNSTypeRepUniversal)
	result.addTypeRep(100, 100, TNSTypeRepUniversal)
	result.addTypeRep(101, 101, TNSTypeRepUniversal)
	result.addTypeRep(TNSDataTypeCursor, TNSDataTypeCursor, TNSTypeRepUniversal)
	result.addTypeRep(TNSDataTypeRDD, types.ROWID, TNSTypeRepUniversal)
	result.addTypeRep(105, 0, TNSTypeRepNative)
	result.addTypeRep(106, 106, TNSTypeRepUniversal)
	result.addTypeRep(TNSDataTypeExtNamed, TNSDataTypeIntNamed, TNSTypeRepUniversal)
	result.addTypeRep(TNSDataTypeIntNamed, TNSDataTypeIntNamed, TNSTypeRepUniversal)
	result.addTypeRep(TNSDataTypeExtRef, TNSDataTypeIntRef, TNSTypeRepUniversal)
	result.addTypeRep(TNSDataTypeIntRef, TNSDataTypeIntRef, TNSTypeRepUniversal)
	result.addTypeRep(TNSDataTypeCLOB, TNSDataTypeCLOB, TNSTypeRepUniversal)
	result.addTypeRep(TNSDataTypeBLOB, TNSDataTypeBLOB, TNSTypeRepUniversal)
	result.addTypeRep(TNSDataTypeBFILE, TNSDataTypeBFILE, TNSTypeRepUniversal)
	result.addTypeRep(TNSDataTypeCFILE, TNSDataTypeCFILE, TNSTypeRepUniversal)
	result.addTypeRep(TNSDataTypeRSET, TNSDataTypeCursor, TNSTypeRepUniversal)
	result.addTypeRep(118, 0, TNSTypeRepNative)
	result.addTypeRep(types.JSONType, types.JSONType, TNSTypeRepUniversal)
	result.addTypeRep(types.VECTOR, types.VECTOR, TNSTypeRepUniversal)
	//if (!OracleConfiguration.MapVectorColumnAsClob)
	//{
	//	TTCDataTypeNegotiation.AddTypeRepresentation(127, 0, 127, 1);
	//}
	result.addTypeRep(121, 0, TNSTypeRepNative)
	result.addTypeRep(122, 0, TNSTypeRepNative)
	result.addTypeRep(123, 0, TNSTypeRepNative)
	result.addTypeRep(136, 0, TNSTypeRepNative)
	result.addTypeRep(146, 146, TNSTypeRepUniversal)
	result.addTypeRep(147, 0, TNSTypeRepNative)
	result.addTypeRep(152, 2, TNSTypeRepOracle)
	result.addTypeRep(153, 2, TNSTypeRepOracle)
	result.addTypeRep(154, 2, TNSTypeRepOracle)
	result.addTypeRep(155, 1, TNSTypeRepUniversal)
	result.addTypeRep(156, 12, TNSTypeRepOracle)
	result.addTypeRep(172, 2, TNSTypeRepOracle)
	result.addTypeRep(178, 178, TNSTypeRepUniversal)
	result.addTypeRep(179, 179, TNSTypeRepUniversal)
	result.addTypeRep(180, 180, TNSTypeRepUniversal)
	result.addTypeRep(181, 181, TNSTypeRepUniversal)
	result.addTypeRep(182, 182, TNSTypeRepUniversal)
	result.addTypeRep(183, 183, TNSTypeRepUniversal)
	result.addTypeRep(184, 12, TNSTypeRepOracle)
	result.addTypeRep(185, 185, TNSTypeRepUniversal)
	result.addTypeRep(186, 186, TNSTypeRepUniversal)
	result.addTypeRep(187, 187, TNSTypeRepUniversal)
	result.addTypeRep(188, 188, TNSTypeRepUniversal)
	result.addTypeRep(189, 189, TNSTypeRepUniversal)
	result.addTypeRep(190, 190, TNSTypeRepUniversal)
	result.addTypeRep(191, 0, TNSTypeRepNative)
	result.addTypeRep(192, 0, TNSTypeRepNative)
	result.addTypeRep(TNSDataTypeDCLOB, TNSDataTypeCLOB, TNSTypeRepUniversal)
	result.addTypeRep(TNSDataTypeDBLOB, TNSDataTypeBLOB, TNSTypeRepUniversal)
	result.addTypeRep(TNSDataTypeDBFILE, TNSDataTypeBFILE, TNSTypeRepUniversal)
	result.addTypeRep(TNSDataTypeUROWID, TNSDataTypeUROWID, TNSTypeRepUniversal)
	result.addTypeRep(209, 0, TNSTypeRepNative)
	result.addTypeRep(198, types.VECTOR, TNSTypeRepUniversal)
	result.addTypeRep(231, 231, TNSTypeRepUniversal)
	result.addTypeRep(232, 231, TNSTypeRepUniversal)
	result.addTypeRep(233, 233, TNSTypeRepUniversal)
	result.addTypeRep(252, 252, TNSTypeRepUniversal)
	result.addTypeRep(241, TNSDataTypeIntNamed, TNSTypeRepUniversal)
	result.addTypeRep(515, 0, TNSTypeRepNative)

	result.DataTypeRepFor1100 = uint16(len(result.TypeAndRep))
	result.addTypeRep(590, 590, TNSTypeRepUniversal)
	result.addTypeRep(591, 591, TNSTypeRepUniversal)
	result.addTypeRep(592, 592, TNSTypeRepUniversal)
	result.addTypeRep(613, 613, TNSTypeRepUniversal)
	result.addTypeRep(614, 614, TNSTypeRepUniversal)
	result.addTypeRep(615, 615, TNSTypeRepUniversal)
	result.addTypeRep(616, 616, TNSTypeRepUniversal)
	result.addTypeRep(647, 647, TNSTypeRepUniversal)
	result.addTypeRep(611, 611, TNSTypeRepUniversal)
	result.addTypeRep(612, 612, TNSTypeRepUniversal)
	result.addTypeRep(617, 617, TNSTypeRepUniversal)
	result.addTypeRep(639, 639, TNSTypeRepUniversal)
	result.addTypeRep(593, 593, TNSTypeRepUniversal)
	result.addTypeRep(594, 594, TNSTypeRepUniversal)
	result.addTypeRep(595, 595, TNSTypeRepUniversal)
	result.addTypeRep(596, 596, TNSTypeRepUniversal)
	result.addTypeRep(597, 597, TNSTypeRepUniversal)
	result.addTypeRep(598, 598, TNSTypeRepUniversal)
	result.addTypeRep(599, 599, TNSTypeRepUniversal)
	result.addTypeRep(600, 600, TNSTypeRepUniversal)
	result.addTypeRep(601, 601, TNSTypeRepUniversal)
	result.addTypeRep(602, 602, TNSTypeRepUniversal)
	result.addTypeRep(603, 603, TNSTypeRepUniversal)
	result.addTypeRep(604, 604, TNSTypeRepUniversal)
	result.addTypeRep(605, 605, TNSTypeRepUniversal)
	result.addTypeRep(622, 622, TNSTypeRepUniversal)
	result.addTypeRep(623, 623, TNSTypeRepUniversal)
	result.addTypeRep(624, 624, TNSTypeRepUniversal)
	result.addTypeRep(625, 625, TNSTypeRepUniversal)
	result.addTypeRep(626, 626, TNSTypeRepUniversal)
	result.addTypeRep(627, 627, TNSTypeRepUniversal)
	result.addTypeRep(628, 628, TNSTypeRepUniversal)
	result.addTypeRep(629, 629, TNSTypeRepUniversal)
	result.addTypeRep(630, 630, TNSTypeRepUniversal)
	result.addTypeRep(631, 631, TNSTypeRepUniversal)
	result.addTypeRep(632, 632, TNSTypeRepUniversal)
	result.addTypeRep(637, 637, TNSTypeRepUniversal)
	result.addTypeRep(638, 638, TNSTypeRepUniversal)
	result.addTypeRep(636, 636, TNSTypeRepUniversal)
	result.DataTypeRepFor1200 = uint16(len(result.TypeAndRep))
	result.addTypeRep(663, 663, TNSTypeRepUniversal)
	result.addTypeRep(640, 640, TNSTypeRepUniversal)
	result.addTypeRep(899, 899, TNSTypeRepUniversal)
	result.addTypeRep(900, 900, TNSTypeRepUniversal)
	result.addTypeRep(901, 901, TNSTypeRepUniversal)
	result.addTypeRep(646, 646, TNSTypeRepUniversal)
	result.addTypeRep(662, 662, TNSTypeRepUniversal)
	if len(result.Server.ServerCompileTimeCaps) > 7 {
		if result.Server.ServerCompileTimeCaps[7] >= 8 {
			result.RuntimeTypeAndRep = result.TypeAndRep
		} else if result.Server.ServerCompileTimeCaps[7] >= 7 {
			result.RuntimeTypeAndRep = result.TypeAndRep[:result.DataTypeRepFor1200]
		} else {
			result.RuntimeTypeAndRep = result.TypeAndRep[:result.DataTypeRepFor1100]
		}
	} else {
		result.RuntimeTypeAndRep = result.TypeAndRep
	}
	return &result
}

func (nego *DataTypeNego) read() (zone *time.Location, err error) {
	var msg uint8
	session := nego.conn.session
	msg, err = session.GetByte()
	if err != nil {
		return
	}
	if msg != 2 {
		err = fmt.Errorf("message code error: received code %d and expected code is 2", msg)
		return
	}
	if nego.RuntimeCap[1] == 1 {
		var tzBytes []byte
		tzBytes, err = session.GetBytes(11)
		if err != nil {
			return
		}
		if len(tzBytes) < 11 {
			err = errors.New("incorrect format for DBTimeZone")
			return
		}
		tzHours := int(tzBytes[4]) - 60
		tzMin := int(tzBytes[5]) - 60
		tzSec := int(tzBytes[6]) - 60
		zone = time.FixedZone(fmt.Sprintf("%+03d:%02d", tzHours, tzMin),
			tzHours*60*60+tzMin*60+tzSec)
		if nego.CompileTimeCaps[37]&2 == 2 {
			nego.serverTZVersion, _ = session.GetInt(4, false, true)
		}
	}
	level := 0
	for {
		var num int
		if nego.CompileTimeCaps[27] == 0 {
			num, err = session.GetInt(1, false, false)
		} else {
			num, err = session.GetInt(2, false, true)
		}
		if num == 0 && level == 0 {
			break
		}
		if num == 0 && level == 1 {
			level = 0
			continue
		}
		if level == 3 {
			level = 0
			continue
		}
		level++
	}
	tracer := nego.conn.tracer
	if zone != nil {
		tracer.Print("DB timezone: ", zone)
	} else {
		tracer.Print("DB timezone not retrieved in data type negotiation")
		tracer.Print("set DB timezone to: UTC(+00:00)")
		nego.conn.dbTimeZone = time.UTC
	}

	nego.conn.session.TTCVersion = nego.conn.dataNego.CompileTimeCaps[7]
	nego.conn.session.UseBigScn = nego.Server.ServerCompileTimeCaps[7] >= 8
	if nego.conn.tcpNego.ServerCompileTimeCaps[7] < nego.conn.session.TTCVersion {
		nego.conn.session.TTCVersion = nego.conn.tcpNego.ServerCompileTimeCaps[7]
	}
	tracer.Print("TTC Version: ", nego.conn.session.TTCVersion)
	if len(nego.conn.tcpNego.ServerRuntimeCaps) > 6 && nego.conn.tcpNego.ServerRuntimeCaps[6]&4 == 4 {
		nego.conn.maxLen.varchar = 0x7FFF
		nego.conn.maxLen.nvarchar = 0x7FFF
		nego.conn.maxLen.raw = 0x7FFF
	} else {
		nego.conn.maxLen.varchar = 0xFA0
		nego.conn.maxLen.nvarchar = 0xFA0
		nego.conn.maxLen.raw = 0xFA0
	}
	return
}

func (nego *DataTypeNego) writeMessage() {
	if nego.Server.ServerCompileTimeCaps != nil && (len(nego.Server.ServerCompileTimeCaps) <= 27 || nego.Server.ServerCompileTimeCaps[27] == 0) {
		nego.CompileTimeCaps[27] = 0
	}
	session := nego.conn.session
	session.PutBytes(nego.MessageCode)
	// client remote in
	// session.PutBytes(0, 0, 0, 0)
	session.PutInt(nego.Server.ServerCharset, 2, false, false)
	// client remote out
	session.PutInt(nego.Server.ServerCharset, 2, false, false)
	session.PutBytes(nego.Server.ServerFlags, uint8(len(nego.CompileTimeCaps)))
	session.PutBytes(nego.CompileTimeCaps...)
	session.PutBytes(uint8(len(nego.RuntimeCap)))
	session.PutBytes(nego.RuntimeCap...)
	if nego.RuntimeCap[1]&1 == 1 {
		session.PutBytes(TZBytes()...)
		if nego.CompileTimeCaps[37]&2 == 2 {
			session.PutInt(nego.clientTZVersion, 4, true, false)
		}
	}
	session.PutInt(nego.Server.ServerNCharset, 2, false, false)
	// marshal type reps
	//size := nego.RuntimeTypeAndRep[0]
	if nego.CompileTimeCaps[27] == 0 {
		for _, x := range nego.RuntimeTypeAndRep {
			session.PutBytes(uint8(x))
		}
		session.PutBytes(0)
	} else {
		for _, x := range nego.RuntimeTypeAndRep {
			session.PutInt(x, 2, true, false)
		}
		session.PutBytes(0, 0)
	}
}
func (nego *DataTypeNego) write() error {
	session := nego.conn.session
	session.ResetBuffer()
	nego.writeMessage()
	return session.Write()
}

func TZBytes() []byte {
	_, offset := time.Now().Zone()
	hours := offset / 3600
	minutes := (offset / 60) % 60
	seconds := offset % 60
	interval, _ := types.NewDaySecondInterval(0, hours, minutes, seconds, 0)
	if interval != nil {
		return interval.Bytes()
	}
	return nil
}
