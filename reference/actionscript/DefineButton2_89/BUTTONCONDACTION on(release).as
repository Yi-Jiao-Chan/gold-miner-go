on(release){
   _parent.bought = 1;
   set("/:stick",eval("/:stick") + 1);
   set("/:score",eval("/:score") - price);
   gotoAndStop(11);
}
