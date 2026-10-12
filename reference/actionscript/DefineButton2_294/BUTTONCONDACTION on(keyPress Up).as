on(keyPress "<Up>"){
   if(eval("/:stick") >= 1)
   {
      set("/:stick",eval("/:stick") - 1);
      _root.dynMov.prevFrame();
      _root.miner.m.gotoAndPlay("throw");
      nextFrame();
   }
}
