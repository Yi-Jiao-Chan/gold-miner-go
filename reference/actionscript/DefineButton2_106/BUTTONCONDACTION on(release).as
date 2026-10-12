on(release){
   _parent.bought = 1;
   set("/:rockBonus",1);
   set("/:score",eval("/:score") - price);
   gotoAndStop(11);
}
