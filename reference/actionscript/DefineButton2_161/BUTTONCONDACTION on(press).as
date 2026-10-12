on(press){
   score = _level0.score;
   gcode = _level0.gcode;
   if(score == 0)
   {
      score = 5;
   }
   getURL("../../www.gamerival.com/game.cfm ","_self","POST");
}
