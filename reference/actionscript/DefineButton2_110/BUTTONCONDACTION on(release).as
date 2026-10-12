on(release){
   _parent.bought = 1;
   set("/:diamondBonus",1);
   set("/:score",eval("/:score") - price);
   gotoAndStop(11);
}
